package main

import (
	"bufio"
	"bytes"
	"encoding/json"
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
	Squeeze bool   `short:"s" long:"squeeze" description:"Squeeze 4:3 video stretched to 16:9 (e.g. Caddx Tarsier 2.7k60)"`
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
	out, err := exec.Command("ffprobe", "-i", opts.Input, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name,width,height,duration,bit_rate", "-print_format", "json").CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	// Parse into struct
	var specs struct {
		Streams []struct {
			Codec    string `json:"codec_name"`
			Width    int
			Height   int
			Duration string
			Bitrate  string `json:"bit_rate"`
		}
	}
	json.Unmarshal(out, &specs)

	// Parse duration to float
	duration, _ := strconv.ParseFloat(specs.Streams[0].Duration, 64)

	// Parse bitrate to int
	if opts.Bitrate == 0 {
		opts.Bitrate, _ = strconv.Atoi(specs.Streams[0].Bitrate)
	}

	var outX int

	if opts.Squeeze {
		outX = specs.Streams[0].Width
	} else {
		outX = int(float64(specs.Streams[0].Height)*(16.0/9.0)) / 2 * 2 // multiplier of 2
	}
	outY := specs.Streams[0].Height

	fmt.Printf("Scaling input file %s (codec: %s, duration: %d secs) from %d*%d to %d*%d using superview scaling. Squeeze: %t\n", opts.Input, specs.Streams[0].Codec, int(duration), specs.Streams[0].Width, specs.Streams[0].Height, outX, outY, opts.Squeeze)

	// Generate PGM P2 files for remap filter, see https://trac.ffmpeg.org/wiki/RemapFilter
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
			sx := float64(x) - float64(outX-specs.Streams[0].Width)/2.0 // x - width diff/2
			tx := (float64(x)/float64(outX) - 0.5) * 2.0                // (x/width - 0.5) * 2

			var offset float64

			if opts.Squeeze {
				inv := 1 - math.Abs(tx)

				offset = inv*(float64((outX/16)*7)/2.0) - math.Pow((inv/16)*7, 2)*(float64((outX/7)*16)/2.0)

				if tx < 0 {
					offset *= -1
				}

				wX.WriteString(strconv.Itoa(int(sx + offset)))
			} else {
				offset = math.Pow(tx, 2) * (float64(outX-specs.Streams[0].Width) / 2.0) // tx^2 * width diff/2

				if tx < 0 {
					offset *= -1
				}

				wX.WriteString(strconv.Itoa(int(sx - offset)))
			}

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
	cmd := exec.Command("ffmpeg", "-hide_banner", "-progress", "pipe:1", "-loglevel", "panic", "-y", "-re", "-i", opts.Input, "-i", "x.pgm", "-i", "y.pgm", "-filter_complex", "remap,format=yuv444p,format=yuv420p", "-c:v", specs.Streams[0].Codec, "-b:v", strconv.Itoa(opts.Bitrate), "-c:a", "aac", "-x265-params", "log-level=error", opts.Output)
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
			fmt.Printf("\rEncoding progress: %.2f%%", math.Min(timeF/(duration*10000), 100))
		}
	}

	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Done! You can open the output file %s to see the result\n", opts.Output)
}
