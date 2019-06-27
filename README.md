# Superview

This is a small Go program that takes a 4:3 aspect ratio video file, and transforms it to a 16:9 video using the  [GoPro SuperView](https://gopro.com/help/articles/question_answer/What-is-SuperView) method. This means that the image is not naively scaled, but a dynamic scaling is applied where the outer areas are stretched more while the center parts stay close to the original aspect rate.

This is not a 1-1 copy of the GoPro algorithm, but an attempt to reach similar quality of output using the open-source [FFmpeg](https://ffmpeg.org/) encoder.

Credits for the idea go to *Banelle*, who wrote the [initial (Python) implementation](https://intofpv.com/t-using-free-command-line-sorcery-to-fake-superview).

Here is a quick animation showing the scaling, note how the text in the center stays the same:

![alt text](sample.gif "Sample of the scaling result")

## Requirements

This program requires FFmpeg in your ``PATH``, please install it using one of these ways:

* Linux: install from your local package manager, for example: ``apt instal ffmpeg``
* Windows: Download from https://ffmpeg.zeranoe.com/builds/
* macOS: Download from https://ffmpeg.zeranoe.com/builds/ or install using Homebrew: ``brew install ffmpeg``

## Installation

Download a recent release from the [releases page on GitHub](https://github.com/Niek/superview/releases). Or build from source using ``go build``.

## Usage

To run the program, launch the ``superview`` program with the ``-i`` (input file) parameter. Optionally, you can provide a ``-o`` (output) and ``-b`` (bitrate) parameter. Full usage instructions:

```
Usage:
  superview [OPTIONS]

Application Options:
  -i, --input=FILE         The input video filename
  -o, --output=FILE        The output video filename (default: output.mp4)
  -b, --bitrate=BITRATE    The bitrate in bytes/second to encode in. If not specified, take the same bitrate as the input file

Help Options:
  -h, --help               Show this help message
```