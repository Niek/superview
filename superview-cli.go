package main

import (
	"fmt"
	"log"
	"os"

	"github.com/jessevdk/go-flags"
)

var opts struct {
	Input   string `short:"i" long:"input" description:"The input video filename" value-name:"FILE" required:"true"`
	Output  string `short:"o" long:"output" description:"The output video filename" value-name:"FILE" required:"false" default:"output.mp4"`
	Codec   string `short:"c" long:"codec" description:"The codec to encode in, either h264 or h265. If not specified, it takes the same codec of the input file" value-name:"[h264|h265]" required:"false"`
	Bitrate int    `short:"b" long:"bitrate" description:"The bitrate in bytes/second to encode in. If not specified, take the same bitrate as the input file" value-name:"BITRATE" required:"false"`
	Squeeze bool   `short:"s" long:"squeeze" description:"Squeeze 4:3 video stretched to 16:9 (e.g. Caddx Tarsier 2.7k60)" required:"false"`
}

func main() {
	// Parse flags
	flags.Parse(&opts)

	_, err := os.Stat(opts.Input)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error opening input file: %s", opts.Input))
		log.Fatal(err)
	}

	ffmpeg, err := checkFfmpeg()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("ffmpeg version: %s\n", ffmpeg["version"])
	fmt.Printf("Hardware accelerators: %s\n", ffmpeg["accels"])
	fmt.Printf("H.264 encoders: %s\n", ffmpeg["h264"])
	fmt.Printf("H.265/HEVC encoders: %s\n", ffmpeg["h265"])

	video, err := checkVideo(opts.Input)
	if err != nil {
		log.Fatal(err)
	}

	// If no bitrate set, use from input video
	if opts.Bitrate == 0 {
		opts.Bitrate = video.Streams[0].BitrateInt
	}

	if opts.Codec == "" {
		opts.Codec = video.Streams[0].Codec
	}
	encoder := findEncoder(opts.Codec, ffmpeg)

	generatePGM(video, opts.Squeeze)

	fmt.Printf("Re-encoding video with %s encoder at %d MB/s bitrate\n", encoder, opts.Bitrate/1024/1024)

	err = encodeVideo(video, encoder, opts.Bitrate, opts.Output, func(v float64) {
		fmt.Printf("\rEncoding progress: %.2f%%", v)
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Done! You can open the output file %s to see the result\n", opts.Output)
}
