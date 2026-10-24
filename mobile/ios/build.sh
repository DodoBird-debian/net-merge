#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
version=${APP_VERSION:-0.4.0}
versionCode=$(python3 -c 'import sys; a,b,c=map(int,sys.argv[1].split(".")); print(a*1000000+b*10000+c*100)' "$version")
mkdir -p Frameworks
(
 cd ../android/golib
 GOBIN="$PWD/.bin" go install golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind
 export PATH="$PWD/.bin:$PATH"
 gomobile bind -v -trimpath -ldflags='-s -w' -target=ios/arm64 -o ../../ios/Frameworks/Netmerge.xcframework ./netmerge
)
file Frameworks/Netmerge.xcframework/ios-arm64/Netmerge.framework/Netmerge
xcodegen generate
xcodebuild -project NetMerge.xcodeproj -scheme NetMerge -configuration Release -sdk iphoneos -destination 'generic/platform=iOS' -derivedDataPath build CODE_SIGNING_ALLOWED=NO MARKETING_VERSION="$version" CURRENT_PROJECT_VERSION="$versionCode" build
mkdir -p build/ipa/Payload ../../dist
cp -R build/Build/Products/Release-iphoneos/NetMerge.app build/ipa/Payload/
(cd build/ipa && zip -qry "../../../../dist/net-merge-$version-ios-arm64-unsigned.ipa" Payload)
