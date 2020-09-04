---
id: introduction
title: Getting Started
---

**NOTE:** Dep was an official experiment to implement a package manager for Go.
As of 2020, Dep is deprecated and archived in favor of Go modules, which have
had official support since Go 1.11. For more details, see https://golang.org/ref/mod.

Welcome! This is documentation for dep, the "official experiment" dependency management tool for the Go language. Dep is a tool intended primarily for use by developers, to support the work of actually writing and shipping code. It is _not_ intended for end users who are installing Go software - that's what `go get` does.

This site has both guides and reference documents. The guides are practical explanations of how to actually do things with dep, whereas the reference material provides deeper dives on specific topics. Of particular note is the [glossary](glossary.md) - if you're unfamiliar with terminology used in this documentation, make sure to check there!

After [installing dep](installation.md), if you're using it for the first time, check out [Creating a New Project](new-project.md). Or, if you have an existing Go project that you want to convert to dep, [Migrating to Dep](migrating.md) is probably the place to start.
