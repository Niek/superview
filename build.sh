#!/bin/sh

if [ $# -ne 1 ]; then
  echo "Usage: ./build.sh <version number>"
  exit
fi

VERSION=$1

echo "Build packages with version number ${VERSION}"

platforms=("windows/amd64" "windows/386" "darwin/amd64" "linux/386" "linux/amd64")

for platform in "${platforms[@]}"
do
    platform_split=(${platform//\// })
    GOOS=${platform_split[0]}
    GOARCH=${platform_split[1]}
    output_name='build/superview-'$GOOS'-'$GOARCH'-'$VERSION
    if [ $GOOS = "windows" ]; then
        output_name+='.exe'
    fi  

    env GOOS=$GOOS GOARCH=$GOARCH go build -o $output_name .
    if [ $? -ne 0 ]; then
        echo 'An error has occurred! Aborting the script execution...'
        exit 1
    fi
done
