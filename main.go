package main

import (
	"bufio"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"log"
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

var outputFileLock sync.Mutex
var reloadOutputFile atomic.Bool

func read(wg *sync.WaitGroup, inputData chan string) {

	defer wg.Done()
	defer close(inputData)

	reader := bufio.NewReader(os.Stdin)

	for {

		// Exit if we read EOF.
		// The only other error ReadString can return happens if the last character
		// is not a delimiter, but thats not an issue for us.
		if text, err := reader.ReadString('\n'); err != nil && err == io.EOF {
			break
		} else {
			inputData <- text
		}
	}
}

func write(wg *sync.WaitGroup, inputData chan string, outputFile string,
	outputFileLock *sync.Mutex, truncateOnStart bool) {

	defer wg.Done()

	// Open output file so we need to take the lock
	outputFileLock.Lock()
	openFlags := os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if truncateOnStart {
		openFlags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	output_file, err := os.OpenFile(outputFile, openFlags, 0644)

	// Fail early: let user know that we cant write to output file
	if err != nil {
		log.Fatalf("Can not write to file %s", outputFile)
	}
	defer output_file.Close()
	outputFileLock.Unlock()

	// Write until the reader closes the input pipe
	for {
		text, ok := <-inputData

		if !ok {
			return
		}

		// Write to output file, we need to take the lock
		outputFileLock.Lock()

		// Check if we need to reopen the output file after rotation
		if reloadOutputFile.Swap(false) {

			// Close current file and reopen
			output_file.Close()
			output_file, err = os.OpenFile(outputFile,
				os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

			// Fail if we cant open the file again...
			if err != nil {
				log.Fatalf("Can not write to file %s", outputFile)
			}
		}

		// Crash if write fails
		if _, err := output_file.WriteString(text); err != nil {
			log.Fatalf("Failed to write to %s", outputFile)
		}
		outputFileLock.Unlock()

		// Write to stdout
		fmt.Print(text)
	}
}

func makeArchiveName(fileName string, index int) string {
	return fileName + "." + strconv.Itoa(index) + ".gz"
}

func nextFreeFileIndex(outputFile string) int {
	i := 1
	for {
		if _, err := os.Stat(makeArchiveName(outputFile, i)); err == nil {
			i += 1
			continue
		}
		return i
	}
}

func gzipFile(inputFilePath string, outputFilePath string) error {

	inputFile, err := os.Open(inputFilePath)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	outputFile, err := os.Create(outputFilePath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	// Read and write gzip file in 4k blocks
	inputBuffer := make([]byte, 4096)

	w := gzip.NewWriter(outputFile)
	defer w.Close()

	for {
		n, err := inputFile.Read(inputBuffer)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		// Only write the number of bytes actually read otherwise
		// we would only write full 4k blocks
		w.Write(inputBuffer[:n])
	}

	return nil
}

func moveOutputFile(outputFile string, outputFileLock *sync.Mutex) error {

	// We are touching the output file so we need the lock
	outputFileLock.Lock()
	defer outputFileLock.Unlock()

	// Move the main log file out of the way
	// The idea is that rename is fast and we want to defer
	// copying / zipping this file so the main writer thread
	// can continue as fast as possible
	if err := os.Rename(outputFile, outputFile+".tmp"); err != nil {
		return err
	}

	// Recreate the output file
	// We do this so a new empty log file is available immediatly
	// If we defer this to the next write there might be no file
	// available until then.
	// If this fails its also not a super big problem...
	if empty, err := os.Create(outputFile); err != nil {
		return nil
	} else {
		empty.Close()
	}

	// Let writer know to open the new output file
	reloadOutputFile.Store(true)
	return nil
}

func rotateFile(outputFile string, outputFileLock *sync.Mutex, maxFiles int, maxAgeDays int) error {

	// Quickly move the output file out of the way so the writer
	// can continue.
	// The rest of the function now has plenty of time - its not blocking anything
	if err := moveOutputFile(outputFile, outputFileLock); err != nil {
		return err
	}

	// Archives are enumerated, find the next free index number
	nextFreeIndex := nextFreeFileIndex(outputFile)

	// Rename oldest log file to free index
	// Bubble this "hole" up
	for i := nextFreeIndex; i > 1; i-- {
		if err := os.Rename(makeArchiveName(outputFile, i-1), makeArchiveName(outputFile, i)); err != nil {
			return err
		}
	}

	// Compress the file we are currently rotating out
	if err := gzipFile(outputFile+".tmp", makeArchiveName(outputFile, 1)); err != nil {
		return err
	}

	// Rotate done, remove temporary file
	os.Remove(outputFile + ".tmp")

	// Apply max files rule
	if maxFiles >= 0 {
		for i := range nextFreeIndex + 1 {
			if i >= maxFiles {

				// Its okay if remove fails here
				if err := os.Remove(makeArchiveName(outputFile, i+1)); err != nil {
					continue
				}
			}
		}
	}

	// Apply file age rule
	if maxAgeDays >= 0 {
		today := time.Now()

		for i := range nextFreeIndex + 1 {
			if stat, err := times.Stat(makeArchiveName(outputFile, i+1)); err == nil {

				// btime might not exist for this OS / FS, if it does not we just continue
				if stat.HasBirthTime() && int(math.Floor(today.Sub(stat.BirthTime()).Hours()/24)) >= maxAgeDays {

					// Its okay if remove fails here
					if err := os.Remove(makeArchiveName(outputFile, i+1)); err != nil {
						continue
					}
				}
			}
		}
	}

	return nil
}

func watchForTrigger(wg *sync.WaitGroup, triggerFilePath string, outputFile string,
	outputFileLock *sync.Mutex, maxFiles int, maxAgeDays int, scanFrequencySeconds float64) {

	for {

		// Tell the wait group that we could exit here before the sleep
		wg.Done()

		// Wait time before checking trigger file
		time.Sleep(time.Millisecond * time.Duration(scanFrequencySeconds*1000))

		// Sleep over, we are actually doing something so we tell the wait group
		// that we can not exit
		wg.Add(1)

		// Check if file containts exactly a single '1'
		// This might explode if someone writes a lot of data to the trigger file...
		if content, err := os.ReadFile(triggerFilePath); err == nil {
			if string(content) != "1" {
				continue
			}
		} else {
			continue
		}

		// Perform rotation, success we write '0' to the trigger file else '2'
		result := "0"
		if err := rotateFile(outputFile, outputFileLock, maxFiles, maxAgeDays); err != nil {
			result = "2"
		}

		// Write the result bit
		// If this fails we have to hard crash, to prevent unintended data loss
		// The trigger file would still contain 1 which would trigger another rotation
		// and failure and so on, rotating all the user data away.
		if err := os.WriteFile(triggerFilePath, []byte(result), 0644); err != nil {
			log.Fatalf("Can not write to %s, shutting down in order to prevent data loss...", triggerFilePath)
		}
	}

}

func main() {

	parser := argparse.NewParser("rotee",
		fmt.Sprintf("tee with integrated logrotate (v %s)", Commit))
	outputFile := parser.String("o", "output-file",
		&argparse.Options{Required: true, Help: "File to redirect output to."})
	triggerFile := parser.String("t", "trigger-file",
		&argparse.Options{Required: false, Help: `Write 1 to this file to trigger logrotate.
		If logrotate succeeds we write '0' to this file, on error we write '2'.
		`})
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

	var wg sync.WaitGroup
	wg.Add(2)
	defer wg.Wait()
	inputData := make(chan string, 30)
	reloadOutputFile.Store(false)

	if triggerFile != nil {
		wg.Add(1)
		go watchForTrigger(&wg, *triggerFile, *outputFile, &outputFileLock, *maxFiles, *maxAgeDays, *scanFrequencySeconds)
	}
	go write(&wg, inputData, *outputFile, &outputFileLock, *truncateOnStart)
	go read(&wg, inputData)

}
