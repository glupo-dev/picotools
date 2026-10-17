#!/bin/sh
# Builds the apps (GL Tools, GL Shell, GL Edit, GL API, GL DB) for Mac and Windows into dist/
# Needs (only on this Mac): Go, Xcode command line tools, mingw-w64 (brew install mingw-w64)
set -e
cd "$(dirname "$0")"
FYNE="go run fyne.io/tools/cmd/fyne@v1.7.3"

rm -rf dist
mkdir -p dist/mac dist/windows

# Each app lives in cmd/<name>; its name, icon and version come from its FyneApp.toml.
for app in gltools glshell gledit glapi gldb; do
	name=$(sed -n 's/^Name = "\(.*\)"/\1/p' "cmd/$app/FyneApp.toml")

	# Mac: <Name>.app
	(cd "cmd/$app" && $FYNE package --os darwin --release)
	mv "cmd/$app/$name.app" dist/mac/

	# Windows: <app>.exe, cross-compiled with mingw-w64
	(cd "cmd/$app" && CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOARCH=amd64 $FYNE package --os windows --release)
	mv "cmd/$app/$name.exe" "dist/windows/$app.exe"
done

ls -lh dist/mac dist/windows
