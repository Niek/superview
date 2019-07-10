#!/bin/sh

if [ $# -ne 1 ]; then
  echo "Usage: ./build.sh <version number>"
  echo "Suggested version: "$(git describe --tags | tr -d v | awk '{printf "%.1f", $1 + .1}')
  exit
fi

VERSION=$1

echo "Build packages with version number ${VERSION}"

platforms=("windows/amd64" "windows/386" "darwin/amd64" "linux/386" "linux/amd64")
files=()

for platform in "${platforms[@]}"
do
    platform_split=(${platform//\// })
    GOOS=${platform_split[0]}
    GOARCH=${platform_split[1]}
    output_name='build/superview-'$GOOS'-'$GOARCH'-v'$VERSION
    if [ $GOOS = "windows" ]; then
        output_name+='.exe'
    fi  

    env GOOS=$GOOS GOARCH=$GOARCH go build -o $output_name .
    if [ $? -ne 0 ]; then
        echo 'An error has occurred! Aborting the script execution...'
        exit 1
    fi

    files+=($output_name)
done

git tag v${VERSION}
git push origin --tags
hub release create -do $(for f in "${files[@]}"; do echo "-a "$f; done) -m "Release v${VERSION}" v${VERSION}