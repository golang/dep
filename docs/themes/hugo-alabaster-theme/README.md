This is a modified theme for golang/dep's doc site prototype, the source is located at https://github.com/carolynvs/hugo-alabaster-theme/tree/depdocs


# Alabaster

A documentation theme ported from [Sphinx](http://www.sphinx-doc.org/en/stable/) to [Hugo](https://gohugo.io).

[![Screenshot](https://raw.githubusercontent.com/digitalcraftsman/hugo-alabaster-theme/dev/images/screenshot.png)](https://digitalcraftsman.github.io/hugo-alabaster-theme/)

## Quick start

Install with `git`:

```sh
git clone git@github.com:digitalcraftsman/hugo-alabaster-theme.git themes/hugo-alabaster-theme
```

> This theme uses the latest developement version of Hugo. Therefore, it doesn't work with the official releases. Look [here](https://github.com/spf13/hugo#build-and-install-the-binaries-from-source-advanced-install) if you want to know how to build Hugo from source.

Next, take a look in the `exampleSite` folder at. This directory contains an example config file and the content for the demo. It serves as an example setup for your documentation.

Copy at least the `config.toml` in the root directory of your website. Overwrite the existing config file if necessary.

Hugo includes a development server, so you can view your changes as you go -
very handy. Spin it up with the following command:

``` sh
hugo server
```

Now you can go to [localhost:1313](http://localhost:1313) and the Alabaster
theme should be visible.

For detailed installation instructions visit the [demo](https://digitalcraftsman.github.io/hugo-alabaster-theme/).

## Acknowledgements

Last but not I want to give a big shout-out to [Jeff Forcier](https://github.com/bitprophet), [Kenneth Reitz](https://github.com/kennethreitz) and [Armin Ronacher](https://github.com/mitsuhiko). Their work and modifications on the original codebase made this port possible.

Furthermore, thanks to [Steve Francia](https://gihub.com/spf13) for creating Hugo and the [awesome community](https://github.com/spf13/hugo/graphs/contributors) around the project.


## License

The theme is released under the BSD license. Read the [license](https://github.com/digitalcraftsman/hugo-alabaster-theme/blob/master/LICENSE.md) for more information.
