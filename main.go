package main

import (
	"bufio"
	"compress/gzip"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/akamensky/argparse"
	"github.com/djherbis/times"
)

//go:generate sh -c "printf %s $(git rev-parse HEAD) > commit.txt"
//go:embed commit.txt
var Commit string

var verbose bool
var outputFileLock sync.Mutex
var reloadOutputFile atomic.Bool

func read(wg *sync.WaitGroup, inputData chan string) {

	defer wg.Done()
	reader := bufio.NewReader(os.Stdin)

	for {
		text, err := reader.ReadString('\n')

		if err != nil {
			if errors.Is(err, io.EOF) {
				close(inputData)
				break
			} else {
				panic(err)
			}
		}
		inputData <- text
	}
}

func write(wg *sync.WaitGroup, inputData chan string, outputFile string, outputFileLock *sync.Mutex, truncateOnStart bool) {

	defer wg.Done()

	outputFileLock.Lock()
	openFlags := os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if truncateOnStart {
		openFlags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	output_file, err := os.OpenFile(outputFile, openFlags, 0644)
	if err != nil {
		panic(err)
	}
	outputFileLock.Unlock()

	defer func() {
		if err := output_file.Close(); err != nil {
			panic(err)
		}
	}()

	for {
		text, ok := <-inputData

		if !ok {
			return
		}

		outputFileLock.Lock()

		if reloadOutputFile.Swap(false) {
			if err := output_file.Close(); err != nil {
				panic(err)
			}

			output_file, err = os.OpenFile(outputFile,
				os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				panic(err)
			}
		}

		if _, err := output_file.WriteString(text); err != nil {
			panic(err)
		}
		outputFileLock.Unlock()
		fmt.Print(text)
	}
}

func nextFreeFilename(outputFile string) int {
	// TODO handle holes
	i := 1
	for {
		if _, err := os.Stat(outputFile + "." + strconv.Itoa(i) + ".gz"); err == nil {
			i += 1
			continue
		}
		return i
	}
}

func gzipFile(inputFilePath string, outputFilePath string) {

	file, _ := os.Open(inputFilePath)

	// close fi on exit and check for its returned error
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()

	fo, err := os.Create(outputFilePath)
	if err != nil {
		panic(err)
	}
	// close fo on exit and check for its returned error
	defer func() {
		if err := fo.Close(); err != nil {
			panic(err)
		}
	}()

	inputBuffer := make([]byte, 4096)

	w := gzip.NewWriter(fo)
	for {
		n, err := file.Read(inputBuffer)
		if err != nil && err != io.EOF {
			panic(err)
		}
		if n == 0 {
			break
		}

		w.Write(inputBuffer[:n])
	}

	w.Close()
}

func rotateFile(outputFile string, outputFileLock *sync.Mutex, maxFiles int, maxAgeDays int) {
	outputFileLock.Lock()
	os.Rename(outputFile, outputFile+".tmp")
	empty, _ := os.Create(outputFile)
	empty.Close()
	reloadOutputFile.Store(true)
	outputFileLock.Unlock()

	nextFreeIndex := nextFreeFilename(outputFile)

	for i := nextFreeIndex; i > 1; i-- {
		if _, err := os.Stat(outputFile + "." + strconv.Itoa(i-1) + ".gz"); err == nil {
			os.Rename(outputFile+"."+strconv.Itoa(i-1)+".gz",
				outputFile+"."+strconv.Itoa(i)+".gz")
		}
	}

	gzipFile(outputFile+".tmp", outputFile+"."+strconv.Itoa(1)+".gz")
	os.Remove(outputFile + ".tmp")

	if maxFiles >= 0 {
		for i := range nextFreeIndex + 1 {
			if i >= maxFiles {
				os.Remove(outputFile + "." + strconv.Itoa(i+1) + ".gz")
			}
		}
	}

	if maxAgeDays >= 0 {
		today := time.Now()
		for i := range nextFreeIndex + 1 {
			if stat, err := times.Stat(outputFile + "." + strconv.Itoa(i+1) + ".gz"); err == nil {
				if stat.HasBirthTime() && int(math.Floor(today.Sub(stat.BirthTime()).Hours()/24)) >= maxAgeDays {
					os.Remove(outputFile + "." + strconv.Itoa(i+1) + ".gz")
				}
			}
		}
	}

}

func watchForTrigger(wg *sync.WaitGroup, triggerFilePath string, outputFile string,
	outputFileLock *sync.Mutex, maxFiles int, maxAgeDays int, scanFrequencySeconds float64) {

	for {
		wg.Done()
		time.Sleep(time.Millisecond * time.Duration(scanFrequencySeconds*1000))
		wg.Add(1)
		file, err := os.Open(triggerFilePath)
		if err != nil {
			continue
		}

		buf := make([]byte, 1)
		_, err = file.Read(buf)
		file.Close()
		if err != nil && err != io.EOF {
			panic(err)
		}
		if buf[0] != '1' {
			continue
		}
		os.WriteFile(triggerFilePath, []byte("0"), 0644)

		rotateFile(outputFile, outputFileLock, maxFiles, maxAgeDays)

	}

}

func main() {

	parser := argparse.NewParser("rotee",
		fmt.Sprintf("tee with integrated logrotate (v %s)", Commit))
	outputFile := parser.String("o", "output-file",
		&argparse.Options{Required: true, Help: "File to redirect output to."})
	triggerFile := parser.String("t", "trigger-file",
		&argparse.Options{Required: false, Help: "Write 1 to this file to trigger logrotate."})
	verboseFlag := parser.Flag("v", "verbose",
		&argparse.Options{Required: false, Help: "Print additional information to stdout", Default: false})
	maxFiles := parser.Int("n", "max-files",
		&argparse.Options{Required: false, Help: "Max number of files to keep. Set to negative number to disable", Default: -1})
	maxAgeDays := parser.Int("d", "max-days",
		&argparse.Options{Required: false,
			Help: "Max age of files to keep in days. Older files are deleted. Set to negative number to disable", Default: -1})
	truncateOnStart := parser.Flag("x", "truncate",
		&argparse.Options{Required: false, Help: "Truncate output file on startup", Default: false})
	scanFrequencySeconds := parser.Float("f", "scan-frequency",
		&argparse.Options{Required: false, Help: "How much time to wait between checking the trigger file in seconds", Default: 1.0})

	// TODO: Disable / Enable compression

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		return
	}

	verbose = *verboseFlag
	var wg sync.WaitGroup
	wg.Add(2)
	defer wg.Wait()
	inputData := make(chan string, 5)

	if triggerFile != nil {
		wg.Add(1)
		go watchForTrigger(&wg, *triggerFile, *outputFile, &outputFileLock, *maxFiles, *maxAgeDays, *scanFrequencySeconds)
	}
	go write(&wg, inputData, *outputFile, &outputFileLock, *truncateOnStart)
	go read(&wg, inputData)

}
