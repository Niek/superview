#!/bin/sh

if [ $# -ne 1 ]; then
    echo "Usage: ./build.sh <version number>"
    echo "Suggested version: "$(git describe --tags | tr -d v | awk '{printf "%.1f", $1 + .1}')
    exit
fi

if ! command -v fyne-cross &> /dev/null; then
    echo "This build script requires fyne-cross v2 to be installed:"
    echo "go get github.com/lucor/fyne-cross/v2/cmd/fyne-cross"
    exit
fi

VERSION=$1

echo "Build packages with version number ${VERSION}"

platforms=("windows/amd64" "windows/386" "darwin/amd64" "linux/386" "linux/amd64")
files=()

for program in "superview-cli" "superview-gui"; do
    for platform in "${platforms[@]}"; do
        platform_split=(${platform//\// })
        GOOS=${platform_split[0]}
        GOARCH=${platform_split[1]}
        output_name="${program}-${GOOS}-${GOARCH}-v${VERSION}"
        if [ $GOOS == "windows" ]; then
            output_name+=".exe"
        fi

        if [ "$program" == "superview-cli" ]; then
            output_name="build/${output_name}"
            env GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -o $output_name ${program}.go
            if [ $? -ne 0 ]; then
                echo "An error has occurred! Aborting the script execution..."
                exit 1
            fi
        else
            fyne-cross ${GOOS} -silent -arch ${GOARCH} -ldflags="-s -w" -output ${output_name} "${program}.go"
            output_name="fyne-cross/dist/${GOOS}-${GOARCH}/${output_name}"
            if [ $GOOS == "windows" ]; then
                output_name+=".zip"
            elif [ $GOOS == "linux" ]; then
                output_name+=".tar.gz"
            else
                if command -v create-dmg &> /dev/null; then
                    create-dmg --hdiutil-quiet --volname "Superview" --volicon "Icon.png" "${output_name}.dmg" "${output_name}.app"
                    output_name+=".dmg"
                else
                    output_name+=".app"
                fi
            fi
        fi

        echo "Built: ${output_name}"

        files+=($output_name)
    done
done

git tag v${VERSION}
git push origin --tags
if command -v hub &> /dev/null; then
    hub release create -do $(for f in "${files[@]}"; do echo "-a "$f; done) -m "Release v${VERSION}" v${VERSION}
fi