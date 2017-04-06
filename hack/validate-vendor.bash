#!/bin/bash
# Copyright 2017 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# This script checks if we changed anything with regard to dependency management
# for our repo and makes sure that it was done in a valid way.

set -e -o pipefail

# TRAVIS_BRANCH is the name of the branch targeted by the pull request.
# but we won't have that set if running locally
if [ -z "$TRAVIS_BRANCH" ]; then
	VALIDATE_REPO='git@github.com:golang/dep.git'
	VALIDATE_BRANCH='master'

	git fetch -q "$VALIDATE_REPO" "refs/heads/$VALIDATE_BRANCH"
	TRAVIS_BRANCH="$(git rev-parse --verify FETCH_HEAD)"
fi

VALIDATE_HEAD="$(git rev-parse --verify HEAD)"

IFS=$'\n'
files=( $(git diff "$TRAVIS_BRANCH...$VALIDATE_HEAD" --diff-filter=ACMR --name-only -- 'Gopkg.toml' 'Gopkg.lock' 'vendor/' || true) )
unset IFS

if [ ${#files[@]} -gt 0 ]; then
	# We run ensure to and see if we have a diff afterwards
	go build
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
