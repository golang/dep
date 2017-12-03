#!/usr/bin/env bash
set -xeuo pipefail

REPO_ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"
DOCS=$REPO_ROOT/docs

# Run the hugo binary in a container, allowing for live-edits of the site's content
hugo () {
  # carolynvs/hugo comes from https://github.com/carolynvs/dockerfiles/blob/master/hugo/Dockerfile
  docker run --rm -it -u $UID -v $DOCS:/var/www -w /var/www -p 1313:1313 carolynvs/hugo:0.30.2 $@
}

# Serve a live preview of the site
preview() {
  hugo server --debug --baseUrl=http://localhost --bind=0.0.0.0 -d _preview
}

# Generate the static site's content for the current version
# master is dumped into the root
# tags are placed under root/releases/VERSION
generate() {
  # Try overridden DEP_VERSION/BRANCH values before retrieving from git
  VERSION=${DEP_VERSION:-$(git describe --exact-match --tags 2> /dev/null || echo "")}
  BRANCH=${DEP_BRANCH:-$(git symbolic-ref --short HEAD 2> /dev/null || echo "")}
  DOCSRC=${VERSION:-$BRANCH}

  if [[ "$VERSION" != "" ]]; then
    DEST=_deploy/releases/$VERSION

    # Start fresh so that removed files are picked up
    if [[ -d $DOCS/$DEST ]]; then
      rm -r $DOCS/$DEST
    fi

    # Set the dep version in the doc's config
    sed -i.bak -e 's/depver = ""/depver = "'"$VERSION"'"/' $DOCS/config.toml
  else
    DEST=_deploy

    # Start fresh so that removed files are picked up
    # Only nuke the main site, don't kill .git or other releases
    if [[ -d $DOCS/$DEST ]]; then
      find $DOCS/$DEST -type f ! -path "*/.git*" ! -path "*/releases*" -print
    fi
  fi

  echo "Generating site @ $DOCSRC into $DEST ..."
  hugo --debug -d $DEST
}

# Generate the current version's docs and push to the gh-pages branch
publish() {
  echo "Cleaning up from previous runs..."
  DEPLOY=$DOCS/_deploy
  if [[ -d $DEPLOY ]]; then
    rm -r $DEPLOY
  fi

  echo "Checking out latest from the gh-pages branch..."
  git clone --branch gh-pages --depth 1 git@github.com:golang/dep.git $DEPLOY

  generate

  pushd $DEPLOY
  if [[ -z "${FORCE:-}" && -z "$(git status --porcelain)" ]]; then
    echo "Skipping site deployment, no changes found"
  else
    echo "Publishing to the gh-pages branch..."
    git config user.name "Travis CI"
    git config user.email "travis@travis-ci.org"
    git add --all
    git commit -m "Automagic site deployment @ $DOCSRC 🎩✨"
    git push
  fi
  popd
}

"$@"
