package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

// VideoSpecs representing a video file
type VideoSpecs struct {
	File    string
	Streams []struct {
		Codec         string `json:"codec_name"`
		Width         int
		Height        int
		Duration      string
		DurationFloat float64
		Bitrate       string `json:"bit_rate"`
		BitrateInt    int
	}
}

// Check for available codecs and hardware accelerators
func checkFfmpeg() (map[string]string, error) {
	ret := make(map[string]string)

	version, err := exec.Command("ffmpeg", "-version").CombinedOutput()

	if err != nil {
		return nil, errors.New("Cannot find ffmpeg/ffprobe on your system.\nMake sure to install it first: https://github.com/Niek/superview/#requirements")
	}

	ret["version"] = strings.Split(string(version), " ")[2]

	// split on newline, skip first line
	accels, err := exec.Command("ffmpeg", "-hwaccels", "-hide_banner").CombinedOutput()
	accelsArr := strings.Split(string(accels), "\n")
	for i := 1; i < len(accelsArr); i++ {
		if len(accelsArr[i]) != 0 {
			ret["accels"] += accelsArr[i] + ","
		}
	}

	// split on newline, skip first 10 lines
	encoders, err := exec.Command("ffmpeg", "-encoders", "-hide_banner").CombinedOutput()
	encodersArr := strings.Split(string(encoders), "\n")
	for i := 10; i < len(encodersArr); i++ {
		if strings.Index(encodersArr[i], " V") == 0 {
			enc := strings.Split(encodersArr[i], " ")
			if strings.Index(enc[2], "h264") == 0 || strings.Index(enc[2], "libx264") == 0 {
				ret["h264"] += enc[2] + ","
			} else if strings.Index(enc[2], "h264") == 0 || strings.Index(enc[2], "libx265") == 0 {
				ret["h265"] += enc[2] + ","
			}
		}
	}

	ret["accels"] = strings.Trim(ret["accels"], ",")
	ret["h264"] = strings.Trim(ret["h264"], ",")
	ret["h265"] = strings.Trim(ret["h265"], ",")

	return ret, nil
}

func checkVideo(file string) (*VideoSpecs, error) {
	// Check specs of the input video (codec, dimensions, duration, bitrate)
	out, err := exec.Command("ffprobe", "-i", file, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name,width,height,duration,bit_rate", "-print_format", "json").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Error running ffprobe, output is:\n%s", out)
	}

	// Parse into struct
	var specs VideoSpecs
	json.Unmarshal(out, &specs)

	// Add input file
	specs.File = file

	// Parse duration to float
	specs.Streams[0].DurationFloat, _ = strconv.ParseFloat(specs.Streams[0].Duration, 64)

	// Parse bitrate to int
	specs.Streams[0].BitrateInt, _ = strconv.Atoi(specs.Streams[0].Bitrate)

	return &specs, nil
}

func generatePGM(video *VideoSpecs, squeeze bool) error {
	var outX int

	if squeeze {
		outX = video.Streams[0].Width
	} else {
		outX = int(float64(video.Streams[0].Height)*(16.0/9.0)) / 2 * 2 // multiplier of 2
	}
	outY := video.Streams[0].Height

	fmt.Printf("Scaling input file %s (codec: %s, duration: %d secs) from %d*%d to %d*%d using superview scaling. Squeeze: %t\n", video.File, video.Streams[0].Codec, int(video.Streams[0].DurationFloat), video.Streams[0].Width, video.Streams[0].Height, outX, outY, squeeze)

	// Generate PGM P2 files for remap filter, see https://trac.ffmpeg.org/wiki/RemapFilter
	fX, err := os.Create("x.pgm")
	if err != nil {
		return err
	}
	fY, err := os.Create("y.pgm")
	if err != nil {
		return err
	}
	defer fX.Close()
	defer fY.Close()

	wX := bufio.NewWriter(fX)
	wY := bufio.NewWriter(fY)

	wX.WriteString(fmt.Sprintf("P2 %d %d 65535\n", outX, outY))
	wY.WriteString(fmt.Sprintf("P2 %d %d 65535\n", outX, outY))

	for y := 0; y < outY; y++ {
		for x := 0; x < outX; x++ {
			sx := float64(x) - float64(outX-video.Streams[0].Width)/2.0 // x - width diff/2
			tx := (float64(x)/float64(outX) - 0.5) * 2.0                // (x/width - 0.5) * 2

			var offset float64

			if squeeze {
				inv := 1 - math.Abs(tx)

				offset = inv*(float64((outX/16)*7)/2.0) - math.Pow((inv/16)*7, 2)*(float64((outX/7)*16)/2.0)

				if tx < 0 {
					offset *= -1
				}

				wX.WriteString(strconv.Itoa(int(sx + offset)))
			} else {
				offset = math.Pow(tx, 2) * (float64(outX-video.Streams[0].Width) / 2.0) // tx^2 * width diff/2

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

	fmt.Println("Filter files generated")

	return nil
}

func findEncoder(codec string, ffmpeg map[string]string) string {
	codec = strings.ToLower(codec)
	encoder := strings.Split(ffmpeg[codec], ",")[0]
	for _, acc := range strings.Split(ffmpeg["accels"], ",") {
		for _, enc := range strings.Split(ffmpeg[codec], ",") {
			if strings.Index(enc, acc) != -1 {
				encoder = enc
			}
		}
	}

	return encoder
}

func encodeVideo(video *VideoSpecs, encoder string, bitrate int, output string, callback func(float64)) error {
	// Starting encoder, write progress to stdout pipe
	cmd := exec.Command("ffmpeg", "-hide_banner", "-progress", "pipe:1", "-loglevel", "panic", "-y", "-re", "-i", video.File, "-i", "x.pgm", "-i", "y.pgm", "-filter_complex", "remap,format=yuv444p,format=yuv420p", "-c:v", encoder, "-b:v", strconv.Itoa(bitrate), "-c:a", "aac", "-x265-params", "log-level=error", output)
	stdout, err := cmd.StdoutPipe()
	rd := bufio.NewReader(stdout)

	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("Error starting ffmpeg, output is:\n%s", stdout)
	}

	// Kill encoder process on Ctrl+C
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigC
		cmd.Process.Kill()
		os.Exit(1)
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
			callback(math.Min(timeF/(video.Streams[0].DurationFloat*10000), 100))
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("Error running ffmpeg, output is:\n%s", stdout)
	}

	return nil
}
