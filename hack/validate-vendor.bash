#!/usr/bin/env bash
# Copyright 2017 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# This script checks if we changed anything with regard to dependency management
# for our repo and makes sure that it was done in a valid way.

set -e -o pipefail

if [ -z "$VALIDATE_UPSTREAM" ]; then
	VALIDATE_REPO='https://github.com/golang/dep.git'
	VALIDATE_BRANCH='master'

	VALIDATE_HEAD="$(git rev-parse --verify HEAD)"

	git fetch -q "$VALIDATE_REPO" "refs/heads/$VALIDATE_BRANCH"
	VALIDATE_UPSTREAM="$(git rev-parse --verify FETCH_HEAD)"

	VALIDATE_COMMIT_DIFF="$VALIDATE_UPSTREAM...$VALIDATE_HEAD"

	validate_diff() {
		if [ "$VALIDATE_UPSTREAM" != "$VALIDATE_HEAD" ]; then
			git diff "$VALIDATE_COMMIT_DIFF" "$@"
		fi
	}
fi

IFS=$'\n'
files=( $(validate_diff --diff-filter=ACMR --name-only -- 'Gopkg.toml' 'Gopkg.lock' 'vendor/' || true) )
unset IFS

if [ ${#files[@]} -gt 0 ]; then
	# This will delete memo section from Gopkg.lock
	# See https://github.com/golang/dep/issues/645 for more info
	# This should go away after -vendor-only flag will be implemented
	# sed -i not used because it works different on MacOS and Linux
	TMP_FILE=`mktemp /tmp/Gopkg.lock.XXXXXXXXXX`
	sed '/memo = \S*/d' Gopkg.lock > $TMP_FILE
	mv $TMP_FILE Gopkg.lock

	# We run ensure to and see if we have a diff afterwards
	go build ./cmd/dep
	./dep ensure
	# Let see if the working directory is clean
	diffs="$(git status --porcelain -- vendor Gopkg.toml Gopkg.lock 2>/dev/null)"
	if [ "$diffs" ]; then
		{
			echo 'The result of ensure differs'
			echo
			echo "$diffs"
			echo
			echo 'Please vendor your package with github.com/golang/dep.'
			echo
		} >&2
		false
	else
		echo 'Congratulations! All vendoring changes are done the right way.'
	fi
else
    echo 'No vendor changes in diff.'
fi
