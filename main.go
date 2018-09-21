package main

import (
	"fmt"
	"io/ioutil"

	"flag"
	"net/http"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/davecgh/go-spew/spew"
	"github.com/gocolly/colly"
	"github.com/julienschmidt/httprouter"

	"github.com/ghodss/yaml"
	"gopkg.in/headzoo/surf.v1"
)

func Index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "Welcome!\n")
}

func Hello(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	fmt.Fprintf(w, "hello, %s!\n", ps.ByName("name"))
}

type Credentials struct {
	Login    string
	Password string
}

type Config struct {
	Forvo Credentials
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

func ForvoSurf(han string) {
	// create browser instance, open login page
	bow := surf.NewBrowser()
	err := bow.Open("https://forvo.com/login/")
	if err != nil {
		panic(err)
	}

	// Print page title
	fmt.Println(bow.Title())

	// Log in to the site.

	// read password/config from config.yaml
	configFileName := "config.yaml"

	raw, err := ioutil.ReadFile(configFileName)
	if err != nil {
		panic(err)
	}

	var config Config
	err = yaml.Unmarshal([]byte(raw), &config)
	if err != nil {
		panic(err)
	}

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
	if err != nil {
		panic(err)
	}

	// go to search results page of word want
	// TODO: error checking ; what if word doesn't exist?
	// TODO: encode characters intelligently?
	//bow.Open("https://forvo.com/search/%E5%85%B3%E7%B3%BB/")
	bow.Open("https://forvo.com/search/" + han + "/")

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

	downloadUrl := "https://forvo.com/download/mp3/" + p2 + "/" + p3 + "/" + p4

	// create file to write to
	// e.g. 关系_12345.mp3
	var file *os.File
	file, err = os.Create(han + "_" + p4 + ".mp3")

	// open download url
	bow.Open(downloadUrl)

	// write to file
	bow.Download(file)
}

/*
	chinesepod:
	err := bow.Open("https://chinesepod.com/accounts/signin")

	// Log in to the site.
	fm, _ := bow.Form("form#signInForm")
	fm.Input("email", "email")
	fm.Input("password", "password")
	if fm.Submit() != nil {
		panic(err)
	}

	// go to search with param string
	err = bow.Open("https://chinesepod.com/tools/glossary/entry/%E6%98%8E%E7%99%BD")

	bow.Open("https://chinesepod.com/redirect/?url=https%3A%2F%2Fs3.amazonaws.com%2Fchinesepod.com%2F0071%2F9ae5499ff3a114baa180e6d10c23d2c9e9233d9b%2Fmp3%2Fglossary%2Fsource%2Frec-1185523902796-3.mp3")

*/

// scrape wiktionary for han, e.g. = 关系
func Wiktionary(han string) Word {
	//func Wiktionary(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	fmt.Println("about to scrape Wiktionary")

	// return value
	var word Word
	word.Definitions = make([]string, 0)
	word.Simplified = ""
	word.Traditional = ""

	// Instantiate default collector
	c := colly.NewCollector(
		// Visit only domains: dict.naver.com
		colly.AllowedDomains("en.wiktionary.org"),
	)

	// If Simplified, follow link for traditional (for actual dict entry)
	c.OnHTML("td span span[class='Hani'] a[href]", func(e *colly.HTMLElement) {
		word.Traditional = e.Text

		// can set Simplified because if we're in this branch, the param is simplified
		word.Simplified = han

		link := e.Request.AbsoluteURL(e.Attr("href"))
		c.Visit(link)
	})

	// on encountering a proper article
	c.OnHTML("#bodyContent", func(e *colly.HTMLElement) {
		sel := e.DOM

		if !strings.Contains(sel.Text(), "For pronunciation and definitions of") {
			// find definitions list items, map each into a brief English definition
			// place resultant slice into Word to return
			word.Definitions = sel.Find("h3 ~ ol li").Map(func(i int, s *goquery.Selection) string {
				// get english definition without below Chinese examples
				return s.Contents().Not("dl").Text()
			})

			// noun, verb, etc.
			// either h3 or h4, depending on if there are multiple pronunciations
			var pos string

			// either e.g. "Pronunciation 1" or e.g. "Noun"
			headlineText := sel.Find("h3 span[class='mw-headline']").First().Text()

			if strings.Contains(headlineText, "Pronunciation 1") {
				pos = sel.Find("h4 span[class='mw-headline']").First().Text()
			} else {
				pos = sel.Find("h3 span[class='mw-headline']").Eq(1).Text()
			}

			word.POS = pos
		}
	})

	// Before making a request print "Visiting ..."
	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting", r.URL.String())
	})

	// Start scraping on wiktionary
	url := fmt.Sprintf("https://en.wiktionary.org/wiki/%s", han)
	c.Visit(url)
	return word
}

func main() {

	var (
		forvo = flag.Bool("forvo", false, "run ForvoSurf()")
		//cp    = flag.Bool("cp", false, "run ChinesePod()")
	)
	flag.Parse()

	if *forvo {
		ForvoSurf("关系")
	}
	/*
		if *cp {
			ChinesePod("关系")
		}
	*/

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
