package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/jessevdk/go-flags"
)

var opts struct {
	Input   string `short:"i" long:"input" description:"The input video filename" value-name:"FILE" required:"true"`
	Output  string `short:"o" long:"output" description:"The output video filename" value-name:"FILE" required:"false" default:"output.mp4"`
	Bitrate int    `short:"b" long:"bitrate" description:"The bitrate in bytes/second to encode in. If not specified, take the same bitrate as the input file" value-name:"BITRATE" required:"false"`
}

func main() {
	// Parse flags
	flags.Parse(&opts)

	_, err := os.Stat(opts.Input)
	if err != nil {
		log.Fatal(err)
	}

	// Check for available codecs
	codecs, err := exec.Command("ffmpeg", "-codecs").CombinedOutput()
	codecsString := string(codecs)

	if err != nil {
		log.Fatal("Cannot find ffmpeg/ffprobe on your system. Make sure to install it first: https://github.com/Niek/superview/#requirements")
	}

	fmt.Printf("ffmpeg version: %s\n", codecsString[strings.Index(codecsString, "ffmpeg version ")+15:20])
	fmt.Printf("H.264 support: %t\n", strings.Contains(codecsString, "H.264"))
	fmt.Printf("H.265/HEVC support: %t\n", strings.Contains(codecsString, "H.265"))

	// Check specs of the input video (codec, dimensions, duration, bitrate)
	out, err := exec.Command("ffprobe", "-i", opts.Input, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name,width,height,duration,bit_rate", "-of", "csv=s=*:p=0").CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	info := strings.Split(strings.TrimSuffix(string(out), "\n"), "*")
	codec := info[0]
	inX, err := strconv.Atoi(info[1])
	inY, err := strconv.Atoi(info[2])
	duration, err := strconv.ParseFloat(info[3], 64)
	bitrate, err := strconv.Atoi(info[4])

	if opts.Bitrate == 0 {
		opts.Bitrate = bitrate
	}

	outX := int(float64(inX)/(4.0/3.0)*(16.0/9.0)) / 2 * 2 // multiplier of 2
	outY := inY

	fmt.Printf("Scaling input file %s (codec: %s, duration: %d secs) from %d*%d to %d*%d using superview scaling\n", opts.Input, codec, int(duration), inX, inY, outX, outY)

	// Generate filter files
	fX, err := os.Create("x.pgm")
	fY, err := os.Create("y.pgm")
	defer fX.Close()
	defer fY.Close()

	wX := bufio.NewWriter(fX)
	wY := bufio.NewWriter(fY)

	wX.WriteString(fmt.Sprintf("P2 %d %d 65535\n", outX, outY))
	wY.WriteString(fmt.Sprintf("P2 %d %d 65535\n", outX, outY))

	for y := 0; y < outY; y++ {
		for x := 0; x < outX; x++ {
			tx := (float64(x)/float64(outX) - 0.5) * 2.0
			sx := float64(x) - float64(outX-inX)/2.0
			offset := math.Pow(tx, 2) * (float64(outX-inX) / 2.0)
			if tx < 0 {
				offset *= -1
			}

			wX.WriteString(strconv.Itoa(int(sx - offset)))
			wX.WriteString(" ")
			wY.WriteString(strconv.Itoa(y))
			wY.WriteString(" ")
		}
		wX.WriteString("\n")
		wY.WriteString("\n")
	}

	wX.Flush()
	wY.Flush()

	fmt.Printf("Filter files generated, re-encoding video at bitrate %d MB/s\n", opts.Bitrate/1024/1024)

	// Starting encoder, write progress to stdout pipe
	cmd := exec.Command("ffmpeg", "-hide_banner", "-progress", "pipe:1", "-loglevel", "panic", "-y", "-re", "-i", opts.Input, "-i", "x.pgm", "-i", "y.pgm", "-filter_complex", "remap,format=yuv444p,format=yuv420p", "-c:v", codec, "-b:v", strconv.Itoa(bitrate), "-c:a", "copy", "-x265-params", "log-level=error", opts.Output)
	stdout, err := cmd.StdoutPipe()
	rd := bufio.NewReader(stdout)

	if err != nil {
		log.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// Kill encoder process on Ctrl+C
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigC
		cmd.Process.Kill()
	}()

	// Read and parse progress
	for {
		line, _, err := rd.ReadLine()

		if err == io.EOF {
			fmt.Printf("\r")
			break
		}

		if bytes.Contains(line, []byte("out_time_ms=")) {
			time := bytes.Replace(line, []byte("out_time_ms="), nil, 1)
			timeF, _ := strconv.ParseFloat(string(time), 64)
			fmt.Printf("\rEncoding progress: %.2f%%", timeF/(duration*10000))
		}
	}

	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Done! You can open the output file %s to see the result\n", opts.Output)
}
