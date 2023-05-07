package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

var filePath string
var threads int

type ListFile struct {
	fp     *os.File
	k      *bufio.Scanner
	closed bool

	lock *sync.Mutex
}

func InitListFile(filePath string) (l ListFile, err error) {
	l.fp, err = os.OpenFile(filePath, os.O_RDONLY, 0400)

	if err == nil {
		l.k = bufio.NewScanner(l.fp)
		l.lock = &sync.Mutex{}
		l.closed = false
	} else {
		l.closed = true
	}

	return
}

func (l *ListFile) ReadLine() (_ string, err error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.closed {
		return "", io.EOF
	}

	if l.k.Scan() {
		return l.k.Text(), nil
	}

	_ = l.fp.Close()
	l.closed = true

	if err = l.k.Err(); err != nil {
		log.Printf("%v", l.k.Err())
	} else {
		err = io.EOF
	}

	return "", err
}

func downloadDocument(url string, fname string) {
	log.Printf("Downloading `%s' into `%s'", url, fname)
	resp, err := http.Get(url)

	if err != nil {
		log.Printf("An error occurred while trying to download `%s': %v", url, err)
		return
	}

	defer deferrable(resp.Body.Close, url)()

	f, err := os.OpenFile(fname, os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("Could not open file `%s': %v", fname, err)
		return
	}

	defer deferrable(f.Close, url)()

	_, err = io.Copy(f, resp.Body)

	log.Printf("Download complete for `%s' into `%s'", url, fname)
}

func deferrable(f func() error, url string) func() {
	return func() {
		if err := f(); err != nil {
			log.Printf("An error occurred (`%s'): %v", url, err)
		}
	}
}

func download(wg *sync.WaitGroup, f *ListFile) {
	defer wg.Done()

	for url, err := f.ReadLine(); err == nil; url, err = f.ReadLine() {
		fname := filenameFromURL(url)

		if fname != "" {
			downloadDocument(url, fname)
		}
	}
}

func filenameFromURL(url string) string {
	urlSplit := strings.Split(url, "/")
	filename := urlSplit[len(urlSplit)-1]
	actualFilename := filename

	_, err := os.Stat(actualFilename)
	for i := 1; err == nil; i++ {
		actualFilename = fmt.Sprintf("%s (%d)", filename, i)
		_, err = os.Stat(actualFilename)
	}

	f, err := os.Create(actualFilename)
	if err != nil {
		log.Printf("Could not create file `%s': %v", actualFilename, err)
		return ""
	}

	_ = f.Close()

	return actualFilename
}

func main() {
	wg := &sync.WaitGroup{}

	flag.StringVar(&filePath, "file", "list.txt", "the path to the file that contains a list of links to resources to download")
	flag.IntVar(&threads, "threads", 1, "number oh concurrent downloads")
	flag.Parse()

	fmt.Printf("Downloading file list: %s\nConcurrent downloads %d\n", filePath, threads)

	f, err := InitListFile(filePath)

	if err != nil {
		log.Fatal(err)
	}

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go download(wg, &f)
	}

	wg.Wait()
}
