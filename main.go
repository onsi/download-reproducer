package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

const PAGE_A = `<!DOCTYPE html>
<html lang="en-US">

<head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width" />
    <title>Page A Testpage</title>
</head>

<body>
	<a href="/page-B" target="_blank" id="open">Open in New Tab</a>
</body>
</html>`

const PAGE_B = `<!DOCTYPE html>
<html lang="en-US">

<head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width" />
    <title>Page B Testpage</title>
</head>

<body>
	Hi!
</body>
</html>`

func pageA(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, PAGE_A)
}
func pageB(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, PAGE_B)
}

func startServer() {
	http.HandleFunc("/page-A", pageA)
	http.HandleFunc("/page-B", pageB)
	go http.ListenAndServe(":8080", nil)
}

func printTab(msg string, tab context.Context) {
	var title string
	chromedp.Run(tab, chromedp.Title(&title))
	ctx := chromedp.FromContext(tab)
	fmt.Printf("%s - %s | ID:%s | Browser ID: %s\n", msg, title, ctx.Target.TargetID, ctx.BrowserContextID)
}

func main() {
	startServer()

	// first we start up a browser instance and grab its websocket url
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir("./"),
	)
	execAllocCtx, cancelAllocCtx := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAllocCtx()
	startBrowserCtx, cancelStartBrowserCtx := chromedp.NewContext(execAllocCtx)
	defer cancelStartBrowserCtx()
	chromedp.Run(startBrowserCtx, chromedp.Evaluate("1", nil))

	bs, _ := os.ReadFile(filepath.Join("./", "DevToolsActivePort"))
	components := strings.Split(string(bs), "\n")
	wsURL := fmt.Sprintf("ws://127.0.0.1:%s%s", components[0], components[1])

	//now we attach to the browser
	allocCtx, cancelAlloCtx := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	defer cancelAlloCtx()

	//we create a couple of new tabs:
	firstTab, cancelFirst := chromedp.NewContext(allocCtx, chromedp.WithNewBrowserContext())
	defer cancelFirst()
	chromedp.Run(firstTab, chromedp.Evaluate("1", nil))

	secondTab, cancelSecond := chromedp.NewContext(firstTab, chromedp.WithNewBrowserContext())
	defer cancelSecond()
	chromedp.Run(secondTab, chromedp.Evaluate("1", nil))

	printTab("First Tab", firstTab)
	printTab("Second Tab", secondTab)

	firstTabId := chromedp.FromContext(firstTab).Target.TargetID

	// now we open a new tab from the first tab
	chromedp.Run(firstTab,
		chromedp.Navigate(`http://localhost:8080/page-A`),
		chromedp.WaitVisible(`body`),
		chromedp.Click(`#open`, chromedp.NodeVisible),
	)
	time.Sleep(time.Second)
	var newTab context.Context
	var cancel context.CancelFunc
	targets, _ := chromedp.Targets(firstTab)
	for _, target := range targets {
		if target.OpenerID == firstTabId {
			newTab, cancel = chromedp.NewContext(firstTab, chromedp.WithTargetID(target.TargetID))
		}
	}
	printTab("New Tab (context based off of firstTab)", newTab)
	cancel()

	//do it again
	chromedp.Run(firstTab,
		chromedp.Navigate(`http://localhost:8080/page-A`),
		chromedp.WaitVisible(`body`),
		chromedp.Click(`#open`, chromedp.NodeVisible),
	)
	time.Sleep(time.Second)
	targets, _ = chromedp.Targets(firstTab)
	for _, target := range targets {
		if target.OpenerID == firstTabId {
			newTab, cancel = chromedp.NewContext(secondTab, chromedp.WithTargetID(target.TargetID)) //this time attach to the secondTab
		}
	}
	printTab("New Tab (context based off of secondTab)", newTab)
	cancel()
}
