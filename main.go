package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
)

const DOWNLOAD_PAGE = `<!DOCTYPE html>
<html lang="en-US">

<head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width" />
    <title>Downloads Testpage</title>
</head>

<body>
    <button id="download">Download</button>
    <script>
        document.getElementById("download").addEventListener("click", () => {
            let anchor = document.createElement('a');
            anchor.setAttribute('href', 'data:text/plain;charset=utf-8,CONTENT');
            anchor.setAttribute('download', 'file.txt');
            anchor.click()
        })
    </script>
</body>
</html>`

func page(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, DOWNLOAD_PAGE)
}

func startServer() {
	http.HandleFunc("/page", page)
	go http.ListenAndServe(":8080", nil)
}

func main() {
	startServer()

	// first we start up a browser instance and grab its websocket url
	fmt.Print("+++ SPINNING UP THE BROWSER\n\n")
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

	//now we attach to the browser twice
	allocCtxA, canceldAlloCtxA := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	defer canceldAlloCtxA()
	allocCtxB, canceldAlloCtxB := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	defer canceldAlloCtxB()

	//we create a new tab on A
	fmt.Print("+++ CREATING FIRST TAB\n\n")
	tabA, cancelA := chromedp.NewContext(allocCtxA, chromedp.WithDebugf(log.Printf))
	defer cancelA()

	//and configure it for downloads
	fmt.Print("+++ CONFIGURING FIRST TAB DOWNLOAD BEHAVIOR\n\n")
	chromedp.Run(tabA, browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorAllowAndName).
		WithDownloadPath("./").
		WithEventsEnabled(true))

	downloads := make(chan string, 1)
	chromedp.ListenTarget(tabA, func(ev interface{}) {
		switch ev := ev.(type) {
		case *browser.EventDownloadWillBegin:
			fmt.Print("\n+++ DOWNLOAD BEGIN\n\n")
		case *browser.EventDownloadProgress:
			switch ev.State {
			case browser.DownloadProgressStateCanceled:
				fmt.Print("\n+++ DOWNLOAD CANCELED\n\n")
				downloads <- "CANCELED"
			case browser.DownloadProgressStateCompleted:
				fmt.Print("\n+++ DOWNLOAD COMPLETE\n\n")
				content, _ := os.ReadFile("./" + ev.GUID)
				downloads <- "COMPLETE: " + string(content)
			}
		}
	})

	//and we try downloading something
	fmt.Print("\n+++ LOADING PAGE AND PERFORMING FIRST DOWNLOAD\n\n")
	chromedp.Run(tabA,
		chromedp.Navigate(`http://localhost:8080/page`),
		chromedp.WaitVisible(`body`),
		chromedp.Click(`#download`, chromedp.NodeVisible),
	)
	firstDownload := <-downloads

	//and we do it again, to prove that we can
	fmt.Print("+++ PERFORMING SECOND DOWNLOAD\n\n")
	chromedp.Run(tabA, chromedp.Click(`#download`, chromedp.NodeVisible))
	secondDownload := <-downloads

	//now we create a new tab in the separate remote allocator
	fmt.Print("+++ CREATING NEW TAB\n\n")
	tabB, cancelB := chromedp.NewContext(allocCtxB, chromedp.WithDebugf(log.Printf))

	//and we configure it for downloads
	fmt.Print("+++ CONFIGURING NEW TAB DOWNLOAD BEHAVIOR\n\n")
	chromedp.Run(tabB, browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorAllowAndName).
		WithDownloadPath("./").
		WithEventsEnabled(true))

	// back on tabA, we try another download
	fmt.Print("\n+++ PERFORMING THIRD DOWNLOAD (ON ORIGINAL TAB)\n\n")
	chromedp.Run(tabA, chromedp.Click(`#download`, chromedp.NodeVisible))
	thirdDownload := <-downloads

	// now we close tab B
	fmt.Print("+++ CLOSING NEW TAB\n\n")
	cancelB()

	// and try one last download on tab A
	fmt.Print("\n+++ PERFORMING FOURTH DOWNLOAD (ON ORIGINAL TAB)\n\n")
	chromedp.Run(tabA, chromedp.Click(`#download`, chromedp.NodeVisible))
	fourthDownload := <-downloads

	fmt.Printf("+++ DOWNLOAD #1: EXPECTED 'COMPLETE: CONTENT', GOT '%s'\n", firstDownload)  // this is fine
	fmt.Printf("+++ DOWNLOAD #2: EXPECTED 'COMPLETE: CONTENT', GOT '%s'\n", secondDownload) // this is fine
	fmt.Printf("+++ DOWNLOAD #3: EXPECTED 'COMPLETE: CONTENT', GOT '%s'\n", thirdDownload)  // this is fine
	fmt.Printf("+++ DOWNLOAD #4: EXPECTED 'COMPLETE: CONTENT', GOT '%s'\n", fourthDownload) // this... is not fine - it shows CANCELED
}
