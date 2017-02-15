// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
)

func main() {
	err := nil
	if err != nil {
		errors.Wrap(err, "thing")
	}
	logrus.Info("whatev")
}
