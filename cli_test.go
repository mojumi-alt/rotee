package main

import (
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testLogFileName string = "test.log"
const testTriggerFileName string = "test.trigger"

func readGzipFile(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gr.Close()

	result, err := io.ReadAll(gr)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func TestTeeOnly(t *testing.T) {

	const testOutputDirectory string = "output_tee"
	const testLines int = 1000

	defer func() {
		if err := os.RemoveAll(testOutputDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Mkdir(testOutputDirectory, 0777); err != nil {
		t.Fatal(err)
	}

	process := exec.Command("./rotee", "-o", filepath.Join(testOutputDirectory, testLogFileName))
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	stdout, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	stderr, err := process.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = process.Start(); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder

	for i := 0; i < testLines; i++ {
		sb.WriteString(strconv.Itoa(i) + ": Text and stuff\n")
	}

	test_input := sb.String()
	if _, err := io.WriteString(stdin, test_input); err != nil {
		t.Fatal(err)
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	output, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatal(err)
	}
	output_string := string(output)

	err_output, err := io.ReadAll(stderr)
	if err != nil {
		t.Fatal(err)
	}
	err_output_string := string(err_output)
	if err := process.Wait(); err != nil {
		t.Fatal(err)
	}

	if output_string != test_input {
		t.Log(output_string)
		t.Fatal("Stdout output missmatch")
	}

	if err_output_string != "" {
		t.Fatal("Stderr has output, should not have any here")
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName)); err != nil || string(log_content) != test_input {
		t.Fatal("Logfile output missmatch")
	}
}

func TestTruncateOnStart(t *testing.T) {

	const testOutputDirectory string = "output_truncate_no_start"
	const subprocessTimeWait int = 50

	defer func() {
		if err := os.RemoveAll(testOutputDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Mkdir(testOutputDirectory, 0777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(testOutputDirectory, testLogFileName), []byte{'a'}, 0644); err != nil {
		t.Fatal(err)
	}

	process := exec.Command("./rotee", "-o", filepath.Join(testOutputDirectory, testLogFileName), "-x")
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = process.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName)); err != nil || string(log_content) != "" {
		t.Fatal("Logfile should be empty")
	}
}

func TestRotate(t *testing.T) {

	const testOutputDirectory string = "output_rotate"
	const iterations int = 7
	const linesPerIteration int = 1000
	const subprocessTimeWait int = 50

	defer func() {
		if err := os.RemoveAll(testOutputDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Mkdir(testOutputDirectory, 0777); err != nil {
		t.Fatal(err)
	}

	process := exec.Command("./rotee",
		"-o", filepath.Join(testOutputDirectory, testLogFileName),
		"-t", filepath.Join(testOutputDirectory, testTriggerFileName),
		"-f", "0.001", "-c",
	)
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = process.Start(); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	var expected []string

	for n := 0; n < iterations; n++ {

		for i := n * linesPerIteration; i < (n+1)*linesPerIteration; i++ {
			sb.WriteString(strconv.Itoa(i) + ": Text and stuff\n")
		}

		test_input := sb.String()
		expected = append(expected, test_input)
		_, err := io.WriteString(stdin, test_input)
		if err != nil {
			t.Fatal(err)
		}

		// Wait for log lines to be processed
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if err := os.WriteFile(filepath.Join(testOutputDirectory, testTriggerFileName), []byte{'1'}, 0644); err != nil {
			t.Fatal(err)
		}

		// Wait for logrotate
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if result, err := os.ReadFile(filepath.Join(testOutputDirectory, testTriggerFileName)); err != nil && string(result) != "0" {
			t.Fatal(err)
		}

		sb.Reset()
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName)); err != nil || string(log_content) != "" {
		t.Fatal("Logfile output missmatch")
	}

	for i, expected_content := range expected {
		if log_content, err := readGzipFile(filepath.Join(testOutputDirectory, testLogFileName+"."+strconv.Itoa(iterations-i)+".gz")); err != nil || string(log_content) != expected_content {
			t.Fatalf("Archive Logfile %d output missmatch", iterations-i)
		}
	}
}

func TestRotateNoCompression(t *testing.T) {

	const testOutputDirectory string = "output_rotate_no_compression"
	const iterations int = 7
	const linesPerIteration int = 1000
	const subprocessTimeWait int = 50

	defer func() {
		if err := os.RemoveAll(testOutputDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Mkdir(testOutputDirectory, 0777); err != nil {
		t.Fatal(err)
	}

	process := exec.Command("./rotee",
		"-o", filepath.Join(testOutputDirectory, testLogFileName),
		"-t", filepath.Join(testOutputDirectory, testTriggerFileName),
		"-f", "0.001",
	)
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = process.Start(); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	var expected []string

	for n := 0; n < iterations; n++ {

		for i := n * linesPerIteration; i < (n+1)*linesPerIteration; i++ {
			sb.WriteString(strconv.Itoa(i) + ": Text and stuff\n")
		}

		test_input := sb.String()
		expected = append(expected, test_input)
		_, err := io.WriteString(stdin, test_input)
		if err != nil {
			t.Fatal(err)
		}

		// Wait for log lines to be processed
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if err := os.WriteFile(filepath.Join(testOutputDirectory, testTriggerFileName), []byte{'1'}, 0644); err != nil {
			t.Fatal(err)
		}

		// Wait for logrotate
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if result, err := os.ReadFile(filepath.Join(testOutputDirectory, testTriggerFileName)); err != nil && string(result) != "0" {
			t.Fatal(err)
		}

		sb.Reset()
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName)); err != nil || string(log_content) != "" {
		t.Fatal("Logfile output missmatch")
	}

	for i, expected_content := range expected {
		if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName+"."+strconv.Itoa(iterations-i))); err != nil || string(log_content) != expected_content {
			t.Fatalf("Archive Logfile %d output missmatch", iterations-i)
		}
	}
}

func TestRotateMaxFiles(t *testing.T) {

	const testOutputDirectory string = "output_rotate_max_files"
	const iterations int = 7
	const linesPerIteration int = 1000
	const subprocessTimeWait int = 50
	const intMaxFiles = 3

	defer func() {
		if err := os.RemoveAll(testOutputDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Mkdir(testOutputDirectory, 0777); err != nil {
		t.Fatal(err)
	}

	process := exec.Command("./rotee",
		"-o", filepath.Join(testOutputDirectory, testLogFileName),
		"-t", filepath.Join(testOutputDirectory, testTriggerFileName),
		"-f", "0.001",
		"-n", strconv.Itoa(intMaxFiles), "-c",
	)
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = process.Start(); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	var expected []string

	for n := 0; n < iterations; n++ {

		for i := n * linesPerIteration; i < (n+1)*linesPerIteration; i++ {
			sb.WriteString(strconv.Itoa(i) + ": Text and stuff\n")
		}

		test_input := sb.String()
		expected = append(expected, test_input)
		_, err := io.WriteString(stdin, test_input)
		if err != nil {
			t.Fatal(err)
		}

		// Wait for log lines to be processed
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if err := os.WriteFile(filepath.Join(testOutputDirectory, testTriggerFileName), []byte{'1'}, 0644); err != nil {
			t.Fatal(err)
		}

		// Wait for logrotate
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if result, err := os.ReadFile(filepath.Join(testOutputDirectory, testTriggerFileName)); err != nil && string(result) != "0" {
			t.Fatal(err)
		}

		sb.Reset()
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName)); err != nil || string(log_content) != "" {
		t.Fatal("Logfile output missmatch")
	}

	for i, expected_content := range expected {
		if i <= intMaxFiles {
			if _, err := os.Stat(filepath.Join(testOutputDirectory, testLogFileName+"."+strconv.Itoa(iterations-i)+".gz")); err == nil {
				t.Fatalf("Archive %d should be deleted", iterations-i)
			}
		} else {

			if log_content, err := readGzipFile(filepath.Join(testOutputDirectory, testLogFileName+"."+strconv.Itoa(iterations-i)+".gz")); err != nil || string(log_content) != expected_content {
				t.Fatalf("Archive Logfile %d output missmatch", iterations-i)
			}
		}
	}
}

func TestRotateMaxAge(t *testing.T) {

	const testOutputDirectory string = "output_rotate_max_age"
	const iterations int = 7
	const linesPerIteration int = 1000
	const subprocessTimeWait int = 50

	defer func() {
		if err := os.RemoveAll(testOutputDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Mkdir(testOutputDirectory, 0777); err != nil {
		t.Fatal(err)
	}

	process := exec.Command("./rotee",
		"-o", filepath.Join(testOutputDirectory, testLogFileName),
		"-t", filepath.Join(testOutputDirectory, testTriggerFileName),
		"-f", "0.001",
		"-d", "0", "-c",
	)
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = process.Start(); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder

	for n := 0; n < iterations; n++ {

		for i := n * linesPerIteration; i < (n+1)*linesPerIteration; i++ {
			sb.WriteString(strconv.Itoa(i) + ": Text and stuff\n")
		}

		test_input := sb.String()
		_, err := io.WriteString(stdin, test_input)
		if err != nil {
			t.Fatal(err)
		}

		// Wait for log lines to be processed
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if err := os.WriteFile(filepath.Join(testOutputDirectory, testTriggerFileName), []byte{'1'}, 0644); err != nil {
			t.Fatal(err)
		}

		// Wait for logrotate
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if result, err := os.ReadFile(filepath.Join(testOutputDirectory, testTriggerFileName)); err != nil && string(result) != "0" {
			t.Fatal(err)
		}

		sb.Reset()
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName)); err != nil || string(log_content) != "" {
		t.Fatal("Logfile output missmatch")
	}

	for i := range iterations {
		if _, err := os.Stat(filepath.Join(testOutputDirectory, testLogFileName+"."+strconv.Itoa(iterations-i)+".gz")); err == nil {
			t.Fatalf("Archive %d should be deleted", iterations-i)
		}
	}
}

func TestRotateNothingLost(t *testing.T) {
	const testOutputDirectory string = "output_rotate_nothing_lost"
	const iterations int = 7
	const linesPerIteration int = 10000
	const subprocessTimeWait int = 100

	defer func() {
		if err := os.RemoveAll(testOutputDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Mkdir(testOutputDirectory, 0777); err != nil {
		t.Fatal(err)
	}

	process := exec.Command("./rotee",
		"-o", filepath.Join(testOutputDirectory, testLogFileName),
		"-t", filepath.Join(testOutputDirectory, testTriggerFileName),
		"-f", "0.001", "-c",
	)
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = process.Start(); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	var expected string

	for n := 0; n < iterations; n++ {

		for i := n * linesPerIteration; i < (n+1)*linesPerIteration; i++ {
			sb.WriteString(strconv.Itoa(i) + ": Text and stuff\n")
		}

		test_input := sb.String()
		expected += test_input
		_, err := io.WriteString(stdin, test_input)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(testOutputDirectory, testTriggerFileName), []byte{'1'}, 0644); err != nil {
			t.Fatal(err)
		}

		if result, err := os.ReadFile(filepath.Join(testOutputDirectory, testTriggerFileName)); err != nil && string(result) != "0" {
			t.Fatal(err)
		}

		sb.Reset()
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	// Wait for writes to complete
	time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

	var all_output string
	for i := range iterations {
		if log_content, err := readGzipFile(filepath.Join(testOutputDirectory, testLogFileName+"."+strconv.Itoa(iterations-i)+".gz")); err != nil {
			continue
		} else {
			all_output += log_content
		}
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName)); err != nil {
		t.Fatal("Logfile could not be read")
	} else {
		all_output += string(log_content)
	}

	if all_output != expected {
		t.Fatal("Output missmatch")
	}
}

func TestPreAndPostScript(t *testing.T) {

	const testOutputDirectory string = "output_pre_and_post_script"
	const linesToWrite int = 1000
	const subprocessTimeWait int = 50
	const preScriptOutputFile = "pre_script_output"
	const postScriptOutputFile = "post_script_output"

	defer func() {
		if err := os.RemoveAll(testOutputDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Mkdir(testOutputDirectory, 0777); err != nil {
		t.Fatal(err)
	}

	expectedPreScriptOutput, err := filepath.Abs(filepath.Join(testOutputDirectory, testLogFileName))
	if err != nil {
		t.Fatal("Could not form pre script output")
	} else {
		expectedPreScriptOutput += "\n"
	}

	expectedPostScriptOutput, err := filepath.Abs(filepath.Join(testOutputDirectory, testLogFileName+".1.gz"))
	if err != nil {
		t.Fatal("Could not form post script output")
	} else {
		expectedPostScriptOutput += "\n"
	}

	process := exec.Command("./rotee",
		"-o", filepath.Join(testOutputDirectory, testLogFileName),
		"-t", filepath.Join(testOutputDirectory, testTriggerFileName),
		"-f", "0.001", "-c",
		"-s", "echo $0 | tee "+filepath.Join(testOutputDirectory, preScriptOutputFile),
		"-p", "echo $0 | tee "+filepath.Join(testOutputDirectory, postScriptOutputFile),
	)
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = process.Start(); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder

	for i := linesToWrite; i < linesToWrite; i++ {
		sb.WriteString(strconv.Itoa(i) + ": Text and stuff\n")
	}

	test_input := sb.String()
	if _, err := io.WriteString(stdin, test_input); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(testOutputDirectory, testTriggerFileName), []byte{'1'}, 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

	if result, err := os.ReadFile(filepath.Join(testOutputDirectory, testTriggerFileName)); err != nil && string(result) != "0" {
		t.Fatal(err)
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, preScriptOutputFile)); err != nil ||
		string(log_content) != expectedPreScriptOutput {
		t.Fatal("Pre script output wrong")
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, postScriptOutputFile)); err != nil ||
		string(log_content) != expectedPostScriptOutput {
		t.Fatal("Post script output wrong")
	}
}

func TestRotateMixedCompression(t *testing.T) {

	const testOutputDirectory string = "output_rotate_mixed_compression"
	const iterations int = 3
	const linesPerIteration int = 100
	const subprocessTimeWait int = 50

	defer func() {
		if err := os.RemoveAll(testOutputDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Mkdir(testOutputDirectory, 0777); err != nil {
		t.Fatal(err)
	}

	process := exec.Command("./rotee",
		"-o", filepath.Join(testOutputDirectory, testLogFileName),
		"-t", filepath.Join(testOutputDirectory, testTriggerFileName),
		"-f", "0.001",
	)
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = process.Start(); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	var expected []string

	for n := 0; n < iterations; n++ {

		for i := n * linesPerIteration; i < (n+1)*linesPerIteration; i++ {
			sb.WriteString(strconv.Itoa(i) + ": Text and stuff\n")
		}

		test_input := sb.String()
		expected = append(expected, test_input)
		_, err := io.WriteString(stdin, test_input)
		if err != nil {
			t.Fatal(err)
		}

		// Wait for log lines to be processed
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if err := os.WriteFile(filepath.Join(testOutputDirectory, testTriggerFileName), []byte{'1'}, 0644); err != nil {
			t.Fatal(err)
		}

		// Wait for logrotate
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if result, err := os.ReadFile(filepath.Join(testOutputDirectory, testTriggerFileName)); err != nil && string(result) != "0" {
			t.Fatal(err)
		}

		sb.Reset()
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	// Okay now we run with compression, the non compressed files have to still be non compressed!
	process = exec.Command("./rotee",
		"-o", filepath.Join(testOutputDirectory, testLogFileName),
		"-t", filepath.Join(testOutputDirectory, testTriggerFileName),
		"-f", "0.001", "-c",
	)
	stdin, err = process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = process.Start(); err != nil {
		t.Fatal(err)
	}

	for n := 0; n < iterations; n++ {

		for i := n * linesPerIteration; i < (n+1)*linesPerIteration; i++ {
			sb.WriteString(strconv.Itoa(i) + ": Text and stuff\n")
		}

		test_input := sb.String()
		expected = append(expected, test_input)
		_, err := io.WriteString(stdin, test_input)
		if err != nil {
			t.Fatal(err)
		}

		// Wait for log lines to be processed
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if err := os.WriteFile(filepath.Join(testOutputDirectory, testTriggerFileName), []byte{'1'}, 0644); err != nil {
			t.Fatal(err)
		}

		// Wait for logrotate
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if result, err := os.ReadFile(filepath.Join(testOutputDirectory, testTriggerFileName)); err != nil && string(result) != "0" {
			t.Fatal(err)
		}

		sb.Reset()
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName)); err != nil || string(log_content) != "" {
		t.Fatal("Logfile output missmatch")
	}

	for i, expected_content := range expected {
		if i < iterations {
			if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName+"."+strconv.Itoa(iterations*2-i))); err != nil || string(log_content) != expected_content {
				t.Fatalf("Archive Logfile %d output missmatch", iterations-i)
			}
		} else {
			if log_content, err := readGzipFile(filepath.Join(testOutputDirectory, testLogFileName+"."+strconv.Itoa(iterations*2-i)+".gz")); err != nil || string(log_content) != expected_content {
				t.Fatalf("Archive Logfile %d output missmatch", iterations-i)
			}
		}
	}
}

func TestRotateTempFileBroken(t *testing.T) {

	const testOutputDirectory string = "output_rotate_temp_file_broken"
	const tempFileName string = testLogFileName + ".tmp"
	const iterations int = 7
	const linesPerIteration int = 1000
	const subprocessTimeWait int = 50

	defer func() {
		if err := os.RemoveAll(testOutputDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Mkdir(testOutputDirectory, 0777); err != nil {
		t.Fatal(err)
	}

	// Place a temp file in the way of rotate
	// This file must not be destroyed by rotate!
	if err := os.WriteFile(filepath.Join(testOutputDirectory, tempFileName), []byte{'a'}, 0644); err != nil {
		t.Fatal(err)
	}

	process := exec.Command("./rotee",
		"-o", filepath.Join(testOutputDirectory, testLogFileName),
		"-t", filepath.Join(testOutputDirectory, testTriggerFileName),
		"-f", "0.001", "-c",
	)
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = process.Start(); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	var expected []string

	for n := 0; n < iterations; n++ {

		for i := n * linesPerIteration; i < (n+1)*linesPerIteration; i++ {
			sb.WriteString(strconv.Itoa(i) + ": Text and stuff\n")
		}

		test_input := sb.String()
		expected = append(expected, test_input)
		_, err := io.WriteString(stdin, test_input)
		if err != nil {
			t.Fatal(err)
		}

		// Wait for log lines to be processed
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if err := os.WriteFile(filepath.Join(testOutputDirectory, testTriggerFileName), []byte{'1'}, 0644); err != nil {
			t.Fatal(err)
		}

		// Wait for logrotate
		// Being slower than this might indicate a problem...
		time.Sleep(time.Millisecond * time.Duration(subprocessTimeWait))

		if result, err := os.ReadFile(filepath.Join(testOutputDirectory, testTriggerFileName)); err != nil && string(result) != "0" {
			t.Fatal(err)
		}

		sb.Reset()
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, testLogFileName)); err != nil || string(log_content) != "" {
		t.Fatal("Logfile output missmatch")
	}

	for i, expected_content := range expected {
		if log_content, err := readGzipFile(filepath.Join(testOutputDirectory, testLogFileName+"."+strconv.Itoa(iterations-i)+".gz")); err != nil || string(log_content) != expected_content {
			t.Fatalf("Archive Logfile %d output missmatch", iterations-i)
		}
	}

	if log_content, err := os.ReadFile(filepath.Join(testOutputDirectory, tempFileName)); err != nil || string(log_content) != "a" {
		t.Fatal("Tempfile was destroyed")
	}
}
