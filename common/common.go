package common

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
func CheckFfmpeg() (map[string]string, error) {
	ret := make(map[string]string)

	cmd := exec.Command("ffmpeg", "-version")
	prepareBackgroundCommand(cmd)
	version, err := cmd.CombinedOutput()

	if err != nil {
		return nil, errors.New("Cannot find ffmpeg/ffprobe on your system.\nMake sure to install it first: https://github.com/Niek/superview/#requirements")
	}

	ret["version"] = strings.Split(string(version), " ")[2]

	// split on newline, skip first line
	cmd = exec.Command("ffmpeg", "-hwaccels", "-hide_banner")
	prepareBackgroundCommand(cmd)
	accels, err := cmd.CombinedOutput()
	accelsArr := strings.Split(strings.ReplaceAll(string(accels), "\r\n", "\n"), "\n")
	for i := 1; i < len(accelsArr); i++ {
		if len(accelsArr[i]) != 0 {
			ret["accels"] += accelsArr[i] + ","
		}
	}

	// split on newline, skip first 10 lines
	cmd = exec.Command("ffmpeg", "-encoders", "-hide_banner")
	prepareBackgroundCommand(cmd)
	encoders, err := cmd.CombinedOutput()
	encodersArr := strings.Split(strings.ReplaceAll(string(encoders), "\r\n", "\n"), "\n")
	for i := 10; i < len(encodersArr); i++ {
		if strings.Index(encodersArr[i], " V") == 0 {
			enc := strings.Split(encodersArr[i], " ")
			if strings.Index(enc[2], "264") != -1 || strings.Index(enc[2], "265") != -1 || strings.Index(enc[2], "hevc") != -1 {
				ret["encoders"] += enc[2] + ","
			}
		}
	}

	ret["accels"] = strings.Trim(ret["accels"], ",")
	ret["encoders"] = strings.Trim(ret["encoders"], ",")

	return ret, nil
}

func GetHeader(ffmpeg map[string]string) string {
	return fmt.Sprintf("- ffmpeg version: %s\n- Hardware accelerators: %s\n- H.264/H.265 encoders: %s\n\n", ffmpeg["version"], ffmpeg["accels"], ffmpeg["encoders"])
}

func CheckVideo(file string) (*VideoSpecs, error) {
	// Check specs of the input video (codec, dimensions, duration, bitrate)
	cmd := exec.Command("ffprobe", "-i", file, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name,width,height,duration,bit_rate", "-print_format", "json")
	prepareBackgroundCommand(cmd)
	out, err := cmd.CombinedOutput()
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

func GeneratePGM(video *VideoSpecs, squeeze bool) error {
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

func FindEncoder(codec string, ffmpeg map[string]string, video *VideoSpecs) string {
	encoder := video.Streams[0].Codec

	if codec != "" {
		for _, enc := range strings.Split(ffmpeg["encoders"], ",") {
			if enc == codec {
				encoder = enc
			}
		}
	}

	return encoder
}

func EncodeVideo(video *VideoSpecs, encoder string, bitrate int, output string, callback func(float64)) error {
	// Starting encoder, write progress to stdout pipe
	cmd := exec.Command("ffmpeg", "-hide_banner", "-progress", "pipe:1", "-loglevel", "panic", "-y", "-re", "-i", video.File, "-i", "x.pgm", "-i", "y.pgm", "-filter_complex", "remap,format=yuv444p,format=yuv420p", "-c:v", encoder, "-b:v", strconv.Itoa(bitrate), "-c:a", "aac", "-x265-params", "log-level=error", output)
	prepareBackgroundCommand(cmd)
	stdout, err := cmd.StdoutPipe()
	rd := bufio.NewReader(stdout)

	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("Error starting ffmpeg, output is:\n%s", err)
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
		return fmt.Errorf("Error running ffmpeg, output is:\n%s", err)
	}

	return nil
}
