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
var dir string

type ListFile struct {
	fp     *os.File
	k      *bufio.Scanner
	closed bool
	lineNo int

	dir string

	lock *sync.Mutex
}

func InitListFile(filePath string, dir string) (l ListFile, err error) {
	l.fp, err = os.OpenFile(filePath, os.O_RDONLY, 0400)

	if err == nil {
		l.k = bufio.NewScanner(l.fp)
		l.lock = &sync.Mutex{}
		l.closed = false
		l.dir = dir
	} else {
		l.closed = true
	}

	return
}

func (f *ListFile) ReadLine() (_ string, _ int, err error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.closed {
		return "", -1, io.EOF
	}

	f.lineNo++

	if f.k.Scan() {
		return f.k.Text(), f.lineNo, nil
	}

	_ = f.fp.Close()
	f.closed = true

	if err = f.k.Err(); err != nil {
		log.Printf("%v", f.k.Err())
	} else {
		err = io.EOF
	}

	return "", f.lineNo, err
}

func (f *ListFile) downloadDocument(url string, fname string, lineNo int) {
	f.Log(os.Stdout, lineNo, "Downloading `%s' into `%s'\n", url, fname)
	resp, err := http.Get(url)

	if err != nil {
		f.Log(os.Stderr, lineNo, "An error occurred while trying to download `%s': %v\n", url, err)
		return
	}

	defer deferrable(resp.Body.Close, url)()

	fp, err := os.OpenFile(fname, os.O_WRONLY, 0600)
	if err != nil {
		f.Log(os.Stderr, lineNo, "Could not open file `%s': %v\n", fname, err)
		return
	}

	defer deferrable(fp.Close, url)()

	_, err = io.Copy(fp, resp.Body)

	f.Log(os.Stdout, lineNo, "Download complete for `%s' into `%s'\n", url, fname)
}

func deferrable(f func() error, url string) func() {
	return func() {
		if err := f(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "An error occurred (`%s'): %v", url, err)
		}
	}
}

func (f *ListFile) download(wg *sync.WaitGroup) {
	defer wg.Done()

	url, lineNo, err := f.ReadLine()
	for err == nil {
		fname := filenameFromURL(f.dir, url)

		if fname != "" {
			f.downloadDocument(url, fname, lineNo)
		} else {
			f.Log(os.Stderr, lineNo, "Could not download `%s': it was impossible to secure a file\n", url)
		}

		url, lineNo, err = f.ReadLine()
	}
}

func (f *ListFile) Log(fp *os.File, line int, format string, args ...interface{}) {
	format = fmt.Sprintf("[#%d] %s", line, format)
	_, _ = fmt.Fprintf(fp, format, args...)
}

var filenameFromURL = func() func(dir, url string) string {
	var m = &sync.Mutex{}

	return func (dir, url string) string {
		m.Lock()
		defer m.Unlock()

		urlSplit := strings.Split(url, "/")
		filename := dir + "/" + urlSplit[len(urlSplit)-1]
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
}()

func secureDir(dir string) error {
	fi, err := os.Stat(dir)

	if err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("`%s' is not a directory", dir)
		}

		return nil
	}

	return os.Mkdir(dir, 0770)
}

func main() {
	wg := &sync.WaitGroup{}

	flag.StringVar(&filePath, "file", "list.txt", "the path to the file that contains a list of links to resources to download")
	flag.IntVar(&threads, "threads", 1, "number oh concurrent downloads")
	flag.StringVar(&dir, "dir", ".", "the directory that files will be downloaded in")
	flag.Parse()

	fmt.Printf("Downloading file list: %s\nConcurrent downloads %d\n", filePath, threads)

	f, err := InitListFile(filePath, dir)
	if err != nil {
		log.Fatal(err)
	}

	err = secureDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go f.download(wg)
	}

	wg.Wait()
}
