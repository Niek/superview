package main

import (
	"fmt"
	"log"
	"strconv"

	"fyne.io/fyne"
	"fyne.io/fyne/app"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/storage"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
)

func main() {
	var video *VideoSpecs

	app := app.New()
	app.Settings().SetTheme(theme.LightTheme())

	window := app.NewWindow("Superview")

	title := widget.NewLabel("Superview - dynamic video stretching")
	title.Alignment = fyne.TextAlignCenter
	title.TextStyle = fyne.TextStyle{Bold: true}

	info := widget.NewMultiLineEntry()
	info.SetReadOnly(true)
	//info.Disable()
	info.SetPlaceHolder("Info box...")

	squeeze := widget.NewCheck("Squeeze video", nil)
	bitrate := widget.NewEntry()
	bitrate.SetPlaceHolder("Set output bitrate in bytes/second if you want to change")

	start := widget.NewButton("Encode the video...", func() {
		dialog.ShowFileSave(func(file fyne.URIWriteCloser, err error) {
			if err == nil && file == nil {
				log.Println("File saving cancelled")
				return
			}
			if err != nil {
				dialog.ShowError(err, window)
				return
			}

			uri := file.URI().String()
			err = file.Close()
			if err != nil {
				fyne.LogError("Failed to close stream", err)
			}

			prog := dialog.NewProgress("Re-encoding", "Please wait...", window)
			prog.Show()

			go func() {
				err := generatePGM(video, squeeze.Checked)
				if err != nil {
					prog.Hide()
					dialog.ShowError(err, window)
					return
				}

				br, err := strconv.Atoi(bitrate.Text)
				if err != nil {
					br = video.Streams[0].BitrateInt
				}

				err = encodeVideo(video, br, uri, func(v float64) { prog.SetValue(v / 100) })
				if err != nil {

					dialog.ShowError(err, window)
					return
				}

				prog.Hide()
				dialog.ShowInformation("Encode done", "Encode finished, your output file can be found here:\n"+uri, window)
			}()
		}, window)
	})
	start.Disable()

	open := widget.NewButton("Open input video...", func() {
		fd := dialog.NewFileOpen(func(file fyne.URIReadCloser, err error) {
			if err == nil && file == nil {
				log.Println("File opening cancelled")
				return
			}
			if err != nil {
				dialog.ShowError(err, window)
				return
			}

			uri := file.URI().String()
			err = file.Close()
			if err != nil {
				fyne.LogError("Failed to close stream", err)
			}

			video, err = checkVideo(uri)
			if err != nil {
				dialog.ShowError(err, window)
				return
			}
			info.SetText(fmt.Sprintf("%s\nFile opened: %s\nInfo: %vx%v px, %s @ %v Mb/s, %.1f secs", info.Text, uri, video.Streams[0].Width, video.Streams[0].Height, video.Streams[0].Codec, video.Streams[0].BitrateInt/1024/1024, video.Streams[0].DurationFloat))
			start.Enable()
		}, window)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".mp4", ".avi", ".MP4", ".AVI"}))
		fd.Show()
	})

	window.SetContent(widget.NewVBox(
		title,
		info,
		layout.NewSpacer(),
		open,
		squeeze,
		bitrate,
		start,
		widget.NewButton("Quit", func() {
			app.Quit()
		}),
	))

	window.Resize(fyne.NewSize(640, 330))

	codecs, err := checkCodecs()
	if err != nil {
		dialog.ShowError(err, window)
		open.Disable()
	}
	info.SetText(fmt.Sprintf("ffmpeg version: %s\nH.264 support: %s\nH.265/HEVC support: %s", codecs["version"], codecs["h264"], codecs["h265"]))

	window.ShowAndRun()
}
