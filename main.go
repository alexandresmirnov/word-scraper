package main

import (
	"fmt"
	"io/ioutil"

	"flag"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/davecgh/go-spew/spew"
	"github.com/julienschmidt/httprouter"

	"github.com/ghodss/yaml"
	"gopkg.in/headzoo/surf.v1"
)

// constants
const CONFIG_FILE = "config.yaml"

func check(err error) {
	if err != nil {
		panic(err)
	}
}

type Credentials struct {
	Login    string
	Password string
}

type Config struct {
	Forvo      Credentials
	ChinesePod Credentials
}

// Parse YAML file into Config
func getConfigFromFile(filename string) Config {
	configFileName := filename

	raw, err := ioutil.ReadFile(configFileName)
	check(err)

	var config Config
	err = yaml.Unmarshal([]byte(raw), &config)
	check(err)

	return config
}

// get config from CONFIG_FILE file
func getConfig() Config {
	return getConfigFromFile(CONFIG_FILE)
}

type Word struct {
	Simplified  string
	Traditional string
	Pinyin      string
	POS         string
	Definitions []string
}

func printWord(word Word) {
	fmt.Printf("simplified: %s\n", word.Simplified)
	fmt.Printf("traditional: %s\n", word.Traditional)
	fmt.Printf("pinyin: %s\n", word.Pinyin)
	fmt.Printf("part of speech: %s\n", word.POS)

	fmt.Println("definitions:")

	for _, def := range word.Definitions {
		fmt.Println(def)
	}
}

func Forvo(han string) {
	// create browser instance, open login page
	bow := surf.NewBrowser()
	err := bow.Open("https://forvo.com/login/")
	check(err)

	// Log in to the site.

	// read config file
	config := getConfig()

	// read forvo credentials
	forvo := config.Forvo

	spew.Dump(forvo)

	login := forvo.Login
	password := forvo.Password

	// Create Form
	loginForm, _ := bow.Form("form[action*=login]")

	loginForm.Input("login", login)
	loginForm.Input("password", password)

	// log in
	err = loginForm.Submit()
	check(err)

	// go to search results page of word want
	// TODO: error checking ; what if word doesn't exist?
	// TODO: encode characters intelligently?
	bow.Open(fmt.Sprintf("https://forvo.com/search/%s/", han))

	// click first pronunciation link
	bow.Click("a.word")

	// find download URL
	// TODO: all browsing through each pronunciation
	// download into temp dir and play them back or something
	downloadLink := bow.Find("span[title*=Download]")

	/*
		data-p1: aHR0cHM6Ly9mb3J2by5jb20vZG93bmxvYWQvbXAzLyNAIy9AI0AvIyNAQA
		data-p2: %E5%85%B3%E7%B3%BB
		data-p3: zh
		data-p4: 3778024

		url: https://forvo.com/download/mp3/{{data-p2}}/{{data-p3}}/{{data-p4}}

	*/

	// data-p1, etc. attributes and exists bool for err-checking
	var (
		p2     string
		p3     string
		p4     string
		exists bool
	)

	p2, exists = downloadLink.Attr("data-p2")
	if !exists {
		fmt.Printf("data-p2 attribute doesn't exist, exiting")
		os.Exit(1)
	}
	p3, exists = downloadLink.Attr("data-p3")
	if !exists {
		fmt.Printf("data-p2 attribute doesn't exist, exiting")
		os.Exit(1)
	}
	p4, exists = downloadLink.Attr("data-p4")
	if !exists {
		fmt.Printf("data-p2 attribute doesn't exist, exiting")
		os.Exit(1)
	}

	downloadUrl := fmt.Sprintf("https://forvo.com/download/mp3/%s/%s/%s/", p2, p3, p4)

	// create file to write to
	// e.g. 关系_12345.mp3
	var file *os.File
	file, err = os.Create(fmt.Sprintf("%s_%s.mp3", han, p4))

	// open download url
	bow.Open(downloadUrl)

	// write to file
	bow.Download(file)
}

func ChinesePod(han string) {
	// create browser instance, open login page
	bow := surf.NewBrowser()
	err := bow.Open("https://chinesepod.com/accounts/signin")
	check(err)

	// Log in to the site.

	// read password/config from config file
	config := getConfig()

	// read credentials
	credentials := config.ChinesePod

	email := credentials.Login
	password := credentials.Password

	// Create Form
	loginForm, _ := bow.Form("form[action*=signin]")

	loginForm.Input("email", email)
	loginForm.Input("password", password)

	// log in
	err = loginForm.Submit()
	check(err)

	// go to search results page of word want
	// TODO: error checking ; what if word doesn't exist?
	// TODO: encode characters intelligently?
	//bow.Open("https://forvo.com/search/%E5%85%B3%E7%B3%BB/")
	err = bow.Open("https://chinesepod.com/tools/glossary/entry/" + han)
	check(err)

	// find download URL
	// TODO: all browsing through each pronunciation
	// download into temp dir and play them back or something
	downloadLink := bow.Find("a[href*=redirect]")

	downloadUrl, exists := downloadLink.Attr("href")
	if !exists {
		fmt.Println("no download link exists on ChinesePod")
		os.Exit(1)
	}

	fmt.Println(downloadUrl)

	// create file to write to
	// e.g. 关系_12345.mp3
	var file *os.File
	file, err = os.Create(han + "_chinesepod.mp3")

	// open download url
	bow.Open("https://chinesepod.com" + downloadUrl)

	// write to file
	bow.Download(file)
}

// scrape wiktionary for han, e.g. = 关系
func Wiktionary(han string) Word {
	//func Wiktionary(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	// return value
	var word Word
	word.Definitions = make([]string, 0)
	word.Simplified = ""
	word.Traditional = ""

	// create browser
	bow := surf.NewBrowser()
	err := bow.Open(fmt.Sprintf("https://en.wiktionary.org/wiki/%s", han))
	check(err)

	// get link to traditional definitions
	traditionalLink := bow.
		Find("td span span[class='Hani'] a[href]").
		FilterFunction(func(i int, sel *goquery.Selection) bool {
			return strings.Contains(sel.Parent().Text(), "For pronunciation")
		})

	// if something matches the above Find, need to navigate to traditional page
	if traditionalLink.Length() > 0 {
		// can set this safely since we know link text contains traditional
		word.Traditional = traditionalLink.Text()

		// can set Simplified because if we're in this branch, the param is simplified
		word.Simplified = han

		// get link to traditional page
		href, exists := traditionalLink.Attr("href")

		// no href attribute means something's wrong
		if !exists {
			fmt.Printf("ERROR: no link to traditionl page")
			os.Exit(1)
		}

		// get URL object from href
		relUrl, err := url.Parse(href)
		check(err)

		// make potentially relative url into absolute
		link := bow.ResolveUrl(relUrl).String()

		// navigate to traditional page defs
		bow.Open(link)

	} else {
		// param is traditional in this branch
		word.Traditional = han
	}

	/*
		TODO: handle all three cases
		1. Query is traditional (simplified is different)
		2. Query is simplified (traditional is different)
		3. Query is both (characters match)

		atm, case 3 not handled
	*/

	// at this point, we are for sure in the traditional defs page

	// find content
	content := bow.Find("#bodyContent")
	if content.Length() == 0 {
		fmt.Printf("ERROR: couldn't find body content")
		os.Exit(1)
	}

	// find pinyin
	// note that I'm just getting the first pronunciation atm (ignoring potential variants)
	word.Pinyin = content.Find("span[class*='pinyin'] a").First().Text()

	// find definitions list items, map each into a brief English definition
	// place resultant slice into Word to return
	// TODO: regex out countable/uncountable and classifier
	word.Definitions = content.Find("h3 ~ ol li").Map(func(i int, s *goquery.Selection) string {
		// get english definition without below Chinese examples
		return s.Contents().Not("dl").Text()
	})

	// noun, verb, etc.
	// either h3 or h4, depending on if there are multiple pronunciations
	var pos string

	// either e.g. "Pronunciation 1" or e.g. "Noun"
	headlineText := content.Find("h3 span[class='mw-headline']").First().Text()

	if strings.Contains(headlineText, "Pronunciation 1") {
		pos = content.Find("h4 span[class='mw-headline']").First().Text()
	} else {
		pos = content.Find("h3 span[class='mw-headline']").Eq(1).Text()
	}

	word.POS = pos

	spew.Dump(word)

	return word
}

func Index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "Welcome!\n")
}

func Hello(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	fmt.Fprintf(w, "hello, %s!\n", ps.ByName("name"))
}

func main() {

	var (
		forvo = flag.Bool("forvo", false, "run Forvo()")
		cp    = flag.Bool("cp", false, "run ChinesePod()")
		wi    = flag.Bool("wi", false, "run Wiktionary()")
	)
	flag.Parse()

	if *forvo {
		Forvo("关系")
	}
	if *cp {
		ChinesePod("关系")
	}
	if *wi {
		//Wiktionary("关系")
		Wiktionary("知道")
	}

	//ChinesePod("关系")

	/*
		guanxi := Wiktionary("关系")

		fmt.Printf("pos: %s\n", guanxi.POS)

		fmt.Println("definitions: ")
		for _, def := range guanxi.Definitions {
			fmt.Println(def)
		}

		fmt.Println()
		fmt.Println()
		fmt.Println()

		zhidao := Wiktionary("知道")

		printWord(zhidao)
	*/

	/*
		fmt.Printf("pos: %s\n", zhidao.POS)

		fmt.Println("definitions: ")
		for _, def := range zhidao.Definitions {
			fmt.Println(def)
		}

		/*
			fmt.Println()
			fmt.Printf("traditional: %s\n", zhidao.Traditional)
			for i, def := range zhidao.Definitions {
				fmt.Printf("\ndefinitions %d: %s\n", i, def)
			}
	*/

	/*
		router := httprouter.New()
		router.GET("/", Index)
		router.GET("/hello/:name", Hello)
		router.GET("/scrape/linedict", LineDict)
		//router.GET("/scrape/wiktionary", Wiktionary)

		log.Fatal(http.ListenAndServe(":8080", router))
	*/
}
