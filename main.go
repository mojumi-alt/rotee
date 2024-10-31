package main

import (
	"bufio"
	"compress/gzip"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/akamensky/argparse"
	"github.com/djherbis/times"
)

type rotateConfig struct {
	maxFiles             int
	maxAgeDays           int
	scanFrequencySeconds float64
	useCompression       bool
	preScript            *string
	postScript           *string
}

type archiveFile struct {
	name       string
	index      int
	compressed bool
}

//go:generate sh -c "printf %s $(git rev-parse --short HEAD) > commit.txt"
//go:embed commit.txt
var Commit string

var outputFileLock sync.Mutex
var rotateLock sync.Mutex
var reloadOutputFile atomic.Bool
var verbose bool

func read(wg *sync.WaitGroup, inputData chan string) {

	logActivity("Reader thread started")
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

	logActivity("Reader thread stopped")
}

func write(wg *sync.WaitGroup, inputData chan string, outputFile string, truncateOnStart bool) {

	logActivity("Writer thread started")
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
		logActivity("Can not write to file %s", outputFile)
		log.Fatalf("Can not write to file %s", outputFile)
	}
	defer output_file.Close()
	outputFileLock.Unlock()

	// Write until the reader closes the input pipe
	for {
		text, ok := <-inputData

		if !ok {
			logActivity("Writer thread stopped")
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

func makeArchivePath(fileName string, index int, compressed bool) string {
	if compressed {
		return fileName + "." + strconv.Itoa(index) + ".gz"
	} else {
		return fileName + "." + strconv.Itoa(index)
	}
}

func (archive *archiveFile) getPath() string {
	return makeArchivePath(archive.name, archive.index, archive.compressed)
}

func findAllArchives(outputFile string) []archiveFile {
	archives := make([]archiveFile, 0)

	// Walk archive files until we get a file not found error
	// This way we know the next free index we can place an archive on
	for i := 1; ; i++ {
		if compressed, err := isArchiveCompressed(outputFile, i); err == nil {
			archives = append(archives, archiveFile{name: outputFile, compressed: compressed, index: i})
		} else {
			return archives
		}
	}
}

func copyFile(inputFilePath string, outputFilePath string) error {

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

	if _, err := io.Copy(outputFile, inputFile); err != nil {
		return err
	}

	return nil
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

	gzipWriter := gzip.NewWriter(outputFile)
	defer gzipWriter.Close()

	if _, err := io.Copy(gzipWriter, inputFile); err != nil {
		return err
	}

	return nil
}

func nextFreeFile(outputFile string) string {
	i := 1
	for {
		if _, err := os.Stat(outputFile + "." + strconv.Itoa(i)); err == nil {
			i += 1
			continue
		}
		return outputFile + "." + strconv.Itoa(i)
	}
}

func moveOutputFile(outputFile string) (string, error) {

	// We are touching the output file so we need the lock
	outputFileLock.Lock()
	defer outputFileLock.Unlock()

	// Move the main log file out of the way
	// The idea is that rename is fast and we want to defer
	// copying / zipping this file so the main writer thread
	// can continue as fast as possible

	// Find a free output filename
	tempOutputFile := nextFreeFile(outputFile + ".tmp")
	if err := os.Rename(outputFile, tempOutputFile); err != nil {
		logActivity("Moved log file to temporary %s", tempOutputFile)
		return tempOutputFile, err
	}

	// Recreate the output file
	// We do this so a new empty log file is available immediatly
	// If we defer this to the next write there might be no file
	// available until then.
	// If this fails its also not a super big problem...
	if empty, err := os.Create(outputFile); err == nil {
		empty.Close()
	}

	// Let writer know to open the new output file
	reloadOutputFile.Store(true)
	return tempOutputFile, nil
}

func isArchiveCompressed(outputFile string, index int) (bool, error) {

	// Archive files can be compressed or non compressed
	// We need to check in what category the file we are looking for is

	// Check if compressed
	if _, err := os.Stat(makeArchivePath(outputFile, index, true)); err == nil {
		return true, nil
	}

	// Input file might be non compressed
	if _, err := os.Stat(makeArchivePath(outputFile, index, false)); err == nil {
		return false, nil
	} else {

		// We cant find the input file
		return false, err
	}
}

func prepend(x []archiveFile, y archiveFile) []archiveFile {
	x = append(x, archiveFile{})
	copy(x[1:], x)
	x[0] = y
	return x
}

func moveArchiveFileUp(archive *archiveFile) error {

	// If target path we want to rotate to exists we stop
	// before overwriting any data...
	inputFile := archive.getPath()
	outputFile := makeArchivePath(archive.name, archive.index+1, archive.compressed)
	if _, err := os.Stat(outputFile); err == nil {
		return errors.New("Rotate target file exists! " + outputFile)
	}
	if err := os.Rename(inputFile, outputFile); err != nil {
		return err
	}

	archive.index += 1
	return nil
}

func rotateFile(outputFile string, config rotateConfig) error {

	// There are multiple threads using this function at the same
	// time potentially, ensure that rotate finishes before we do another.
	logActivity("Starting logrotate...")
	rotateLock.Lock()
	defer rotateLock.Unlock()

	// Quickly move the output file out of the way so the writer
	// can continue.
	// The rest of the function now has plenty of time - its not blocking anything
	tempOutputFile, err := moveOutputFile(outputFile)
	if err != nil {
		return err
	}

	// Apply pre script if there is one
	if config.preScript != nil && *config.preScript != "" {

		// Obtain abs path to the file the pre script is supposed to operate on
		// If we fail to make abs path just dont run the pre scipt, something is weird...
		if preScriptOperatorFile, err := filepath.Abs(outputFile); err == nil {

			// Run user script, pass output file as arg
			process := exec.Command("/bin/sh", "-c", *config.preScript, preScriptOperatorFile)

			// Run process
			logActivity("Running user defined pre script...")
			if err := process.Run(); err != nil {
				logActivity("Error while running user defined pre script!")
				return err
			}

			// Sanity check that the user script did not delete the output file
			if _, err := os.Stat(tempOutputFile); err != nil {

				// We cant stat the file, assume that something evil
				// happened and error out...
				logActivity("Can not find logfile after user script. Aborting...")
				return err
			}
		} else {
			logActivity("Can not find path to logfile. Error: %s", err)
			return err
		}
	}

	// Move all archive files up by 1
	// Bubble this "hole" up, so there is no .1.gz archive
	logActivity("Moving archives up...")
	archives := findAllArchives(outputFile)
	logActivity("Have %d archives", len(archives))
	for i := len(archives) - 1; i >= 0; i-- {
		if err := moveArchiveFileUp(&archives[i]); err != nil {
			logActivity("Error while moving archive files: %s", err)
			return err
		}
	}

	// Compress / copy the file we are currently rotating out
	newArchive := archiveFile{outputFile, 1, config.useCompression}
	if config.useCompression {
		if err := gzipFile(tempOutputFile, newArchive.getPath()); err != nil {
			logActivity("Error while gziping logfile: %s", err)
			return err
		}
	} else {
		if err := copyFile(tempOutputFile, newArchive.getPath()); err != nil {
			logActivity("Error while copying logfile: %s", err)
			return err
		}
	}
	archives = prepend(archives, newArchive)

	// Rotate done, remove temporary file
	logActivity("Removing temporary logfile...")
	os.Remove(tempOutputFile)

	// Apply post script if there is one
	// We do this before applying delete rules.
	if config.postScript != nil && *config.postScript != "" {

		// Obtain abs path to the file the post script is supposed to operate on
		// If we fail to make abs path just dont run the pre scipt, something is weird...
		logActivity("Running user defined post script...")
		if postScriptOperatorFile, err := filepath.Abs(newArchive.getPath()); err == nil {

			// Run user script, pass archive file name
			process := exec.Command("/bin/sh", "-c", *config.postScript, postScriptOperatorFile)

			// Run process
			if err := process.Run(); err != nil {
				logActivity("Error while running user defined post script!")
				return err
			}
		} else {
			logActivity("Can not find path to logfile. Error: %s", err)
			return err
		}
	}

	// Apply max files rule
	if config.maxFiles >= 0 {
		logActivity("Limit max number of archives to %d", config.maxFiles)
		for i, archive := range archives {
			if i >= config.maxFiles {

				// Its okay if remove fails here
				if err := os.Remove(archive.getPath()); err != nil {
					logActivity("Failed to delete %s", archive.getPath())
					continue
				}
			}
		}
	}

	// Apply file age rule
	if config.maxAgeDays >= 0 {
		logActivity("Limit max number of archives to %d days", config.maxAgeDays)

		today := time.Now()

		for _, archive := range archives {
			if stat, err := times.Stat(archive.getPath()); err == nil {

				// btime might not exist for this OS / FS, if it does not we just continue
				if stat.HasBirthTime() && int(math.Floor(today.Sub(stat.BirthTime()).Hours()/24)) >= config.maxAgeDays {

					// Its okay if remove fails here
					if err := os.Remove(archive.getPath()); err != nil {
						logActivity("Failed to delete %s", archive.getPath())
						continue
					}
				} else {
					logActivity("Cant determine btime of file %s", archive.getPath())
				}
			} else {
				logActivity("Failed to stat %s", archive.getPath())
			}
		}
	}

	return nil
}

func shouldTrigger(triggerFile string) bool {

	// Check if file containts exactly a single '1'
	// We are generous and allow a newline after the '1'
	// This might explode if someone writes a lot of data to the trigger file...
	if content, err := os.ReadFile(triggerFile); err == nil {
		string_content := string(content)
		return string_content == "1\n" || string_content == "1" || string_content == "1\r\n"
	}

	return false
}

func watchForTrigger(wg *sync.WaitGroup, outputFile string, triggerFile string, config rotateConfig) {

	logActivity("Tracking trigger file %s", triggerFile)
	for {

		// Start work, tell wait group that we are busy and cant exit.
		wg.Add(1)

		// Check if trigger files meets conditions to initiate rotate
		if shouldTrigger(triggerFile) {

			// Perform rotation, success we write '0' to the trigger file else '2'
			logActivity("Starting rotate because of trigger file %s", triggerFile)
			result := "0"
			if err := rotateFile(outputFile, config); err != nil {
				result = "2"
			}
			logActivity("Writing status %s to %s", result, triggerFile)

			// Write the result bit
			// If this fails we have to hard crash, to prevent unintended data loss
			// The trigger file would still contain 1 which would trigger another rotation
			// and failure and so on, rotating all the user data away.
			if err := os.WriteFile(triggerFile, []byte(result), 0644); err != nil {
				logActivity("Can not write to %s, shutting down in order to prevent data loss...", triggerFile)
				log.Fatalf("Can not write to %s, shutting down in order to prevent data loss...", triggerFile)
			}
		}

		// Tell the wait group that we could exit here while we are asleep.
		wg.Done()

		// Wait time before checking trigger file
		time.Sleep(time.Millisecond * time.Duration(config.scanFrequencySeconds*1000))
	}
}

func automaticTimedRotation(wg *sync.WaitGroup, autoRotateFrequency float64, outputFile string, config rotateConfig) {

	logActivity("Running logrotate every %f seconds", autoRotateFrequency)
	for {
		// Tell the wait group that we could exit here before the sleep
		wg.Done()

		// Wait time before doing rotate
		time.Sleep(time.Millisecond * time.Duration(autoRotateFrequency*1000))

		// Sleep over, we are actually doing something so we tell the wait group
		// that we can not exit
		wg.Add(1)

		if err := rotateFile(outputFile, config); err != nil {
			logActivity("Timed rotate failed!")
			log.Fatal("Timed rotate failed!")
		}
	}
}

func automaticFileSizeRotation(wg *sync.WaitGroup, maxFileSizeBytes int, outputFile string, config rotateConfig) {

	logActivity("Running logrotate once file has size %d, checking every %f seconds",
		maxFileSizeBytes, config.scanFrequencySeconds)
	for {

		// Start work, tell wait group that we are busy and cant exit.
		wg.Add(1)

		if stat, err := os.Stat(outputFile); err == nil && stat.Size() >= int64(maxFileSizeBytes) {
			if err := rotateFile(outputFile, config); err != nil {
				logActivity("Filed size based rotation failed!")
				log.Fatal("Filed size based rotation failed!")
			}
		} else {
			logActivity("Filed size based rotation could not stat file %s", outputFile)
		}

		// Tell the wait group that we could exit here while we are asleep.
		wg.Done()

		// Wait time before checking file size
		time.Sleep(time.Millisecond * time.Duration(config.scanFrequencySeconds*1000))
	}
}

func logActivity(message string, v ...any) {
	if verbose {
		log.Printf(message, v...)
	}
}

func main() {

	parser := argparse.NewParser("rotee",
		fmt.Sprintf("tee with integrated logrotate (rev: %s)", Commit))
	outputFile := parser.String("o", "output-file",
		&argparse.Options{Required: true, Help: "File to redirect output to."})
	triggerFile := parser.String("t", "trigger-file",
		&argparse.Options{Required: false, Help: "Write 1 to this file to trigger logrotate." +
			"If logrotate succeeds we write '0' to this file, on error we write '2'."})
	maxFiles := parser.Int("n", "max-files",
		&argparse.Options{Required: false, Help: "Max number of files to keep." +
			"Set to negative number to disable." +
			"This rule is applied independently of the max-days rule", Default: -1})
	maxAgeDays := parser.Int("d", "max-days",
		&argparse.Options{Required: false,
			Help: "Max age of files to keep in days." +
				"Older files are deleted. Set to negative number to disable" +
				"This rule is applied independently of the max-files rule", Default: -1})
	truncateOnStart := parser.Flag("x", "truncate",
		&argparse.Options{Required: false, Help: "Truncate output file on startup", Default: false})
	scanFrequencySeconds := parser.Float("f", "scan-frequency",
		&argparse.Options{Required: false, Help: "How much time to wait between checking the trigger file in seconds", Default: 1.0})
	useCompression := parser.Flag("c", "compress",
		&argparse.Options{Required: false, Help: "Whether to compress the output", Default: false})
	preScript := parser.String("s", "pre-script",
		&argparse.Options{Required: false, Help: "Script to run before rotate, " +
			"passes the absolute path to the file about to be rotated to the script"})
	postScript := parser.String("p", "post-script",
		&argparse.Options{Required: false, Help: "Script to run after rotate, " +
			"passes the absolute path to the rotated file to the script"})
	autoRotateFrequency := parser.Float("a", "auto-rotate-frequency",
		&argparse.Options{Required: false, Help: "How long to wait between rotating the file." +
			"Set to a positive number of seconds to activate", Default: -1.0})
	maxLogFileSize := parser.Int("m", "max-logfile-size",
		&argparse.Options{Required: false, Help: "Max logfile size before triggering logrotate." +
			"Set to a positive number of bytes to activate", Default: -1})
	activityFilePath := parser.String("v", "verbose-output-file",
		&argparse.Options{Required: false, Help: "Specify an output file for activity logging"})

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		return
	}

	if *activityFilePath != "" {
		if f, err := os.OpenFile(*activityFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
			log.Fatalf("Cant open activity log file at %s", *activityFilePath)
		} else {
			defer f.Close()
			log.SetOutput(f)
			verbose = true
		}
	}

	// Set up a wait group to prevent shutting down before all writes
	// and rotates are complete.
	var wg sync.WaitGroup
	wg.Add(2)
	defer wg.Wait()

	// Set up channel between reader and writer and initialize
	// logfile reload flag.
	inputData := make(chan string, 50)
	reloadOutputFile.Store(false)

	config := rotateConfig{
		maxFiles:             *maxFiles,
		maxAgeDays:           *maxAgeDays,
		scanFrequencySeconds: *scanFrequencySeconds,
		useCompression:       *useCompression,
		preScript:            preScript,
		postScript:           postScript,
	}

	// Start the desired rotate trigger processes
	if *autoRotateFrequency > 0 {

		// This function does not instantly do a rotate check
		// Instead it starts on sleep so we need to inform the wait group.
		wg.Add(1)
		go automaticTimedRotation(&wg, *autoRotateFrequency, *outputFile, config)
	}

	if *maxLogFileSize > 0 {
		go automaticFileSizeRotation(&wg, *maxLogFileSize, *outputFile, config)
	}

	if *triggerFile != "" {
		go watchForTrigger(&wg, *outputFile, *triggerFile, config)
	}

	// Start reading and writing last.
	go write(&wg, inputData, *outputFile, *truncateOnStart)
	go read(&wg, inputData)

}
