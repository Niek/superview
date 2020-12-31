package main

import (
	"fmt"
	"log"
	"os"
	"superview/common"

	"github.com/jessevdk/go-flags"
)

var opts struct {
	Input   string `short:"i" long:"input" description:"The input video filename" value-name:"FILE" required:"true"`
	Output  string `short:"o" long:"output" description:"The output video filename" value-name:"FILE" required:"false" default:"output.mp4"`
	Encoder string `short:"e" long:"encoder" description:"The encoder to use, use -h to see a list. If not specified, it takes the standard encoder of the input file codec" value-name:"ENCODER" required:"false"`
	Bitrate int    `short:"b" long:"bitrate" description:"The bitrate in bytes/second to encode in. If not specified, take the same bitrate as the input file" value-name:"BITRATE" required:"false"`
	Squeeze bool   `short:"s" long:"squeeze" description:"Squeeze 4:3 video stretched to 16:9 (e.g. Caddx Tarsier 2.7k60)" required:"false"`
}

func main() {
	fmt.Println("===> Superview - dynamic video stretching <===\n")

	// Check for ffmpeg
	ffmpeg, err := common.CheckFfmpeg()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Print(common.GetHeader(ffmpeg))

	// Parse flags
	_, err = flags.Parse(&opts)
	if err != nil {
		os.Exit(0)
	}

	_, err = os.Stat(opts.Input)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error opening input file: %s", opts.Input))
		log.Fatal(err)
	}

	video, err := common.CheckVideo(opts.Input)
	if err != nil {
		log.Fatal(err)
	}

	// If no bitrate set, use from input video
	if opts.Bitrate == 0 {
		opts.Bitrate = video.Streams[0].BitrateInt
	}

	opts.Encoder = common.FindEncoder(opts.Encoder, ffmpeg, video)

	common.GeneratePGM(video, opts.Squeeze)

	fmt.Printf("Re-encoding video with %s encoder at %d MB/s bitrate\n", opts.Encoder, opts.Bitrate/1024/1024)

	err = common.EncodeVideo(video, opts.Encoder, opts.Bitrate, opts.Output, func(v float64) {
		fmt.Printf("\rEncoding progress: %.2f%%", v)
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := common.CleanUp(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Done! You can open the output file %s to see the result\n", opts.Output)
}
