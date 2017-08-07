# golang/dep Integration Tests

The `dep` integration tests use a consistent directory structure under `testdata`
to set up the initial project state, run `dep` commands, then check against an
expected final state to see if the test passes.

The directory structure is as follows:

    testdata/
        harness_tests/
            category1/
                subcategory1/
                    case1/
                        testcase.json
                        stdout.txt
                        initial/
                            file1.go
                            Gopkg.toml
                            ...
                        final/
                            Gopkg.toml
                            Gopkg.lock
                    case2/
                    ...

The test code itself simply walks down the directory tree, looking for
`testcase.json` files.  These files can be as many levels down the tree as
desired.  The test name will consist of the directory path from `testdata` to
the test case directory itself.  In the above example, the test name would be
`category1/subcategory1/case1`, and could be singled out with the `-run` option
of `go test` (i.e.
`go test github.com/golang/dep/cmd/dep -run Integration/category1/subcategory1/case1`).
New tests can be added simply by adding a new directory with the json file to
the `testdata` tree.  There is no need for code modification - the new test
will be included automatically.

The json file needs to be accompanied by `initial` and `final` directories. The
`initial` is copied verbatim into the test project before the `dep` commands are
run, and the `manifest` and `lock` files in `final`, if present, are used to
compare against the test project results after the commands. The `stdout.txt` file
is optional, and if present will be compared with command output.

The `testcase.json` file has the following format:

    {
      "commands": [
        ["init"],
        ["ensure", "github.com/sdboyer/deptesttres"]
      ],
      "gopath-initial": {
        "github.com/sdboyer/deptest": "v0.8.0",
        "github.com/sdboyer/deptestdos": "a0196baa11ea047dd65037287451d36b861b00ea"
      },
      "vendor-initial": {
        "github.com/sdboyer/deptesttres": "v2.1.0",
        "github.com/sdboyer/deptestquatro": "cf596baa11ea047ddf8797287451d36b861bab45"
      },
      "vendor-final": [
        "github.com/sdboyer/deptest",
        "github.com/sdboyer/deptestdos",
        "github.com/sdboyer/deptesttres",
        "github.com/sdboyer/deptestquatro"
      ],
      "error-expected": "something went wrong"
    }

All of the categories are optional - if the `imports` list for a test is empty,
for example, it can be completely left out.

The test procedure is as follows:

1. Create a unique temporary directory (TMPDIR) as the test run's `GOPATH`
2. Create `$TMPDIR/src/github.com/golang/notexist` as the current project
3. Copy the contents of `initial` input directory to the project
4. Fetch the repos and versions in `gopath-initial` into `$TMPDIR/src` directory
5. Fetch the repos and versions in `vendor-initial` to the project's `vendor` directory
6. Run `commands` on the project, in declaration order
7. Ensure that, if any errors are raised, it is only by the final command and their string output matches `error-expected`
8. Ensure that, if a stdout.txt file is present, the command's output matches (excluding trailing whitespace).
9. Check the resulting files against those in the `final` input directory
10. Check the `vendor` directory for the projects listed under `vendor-final`
11. Check that there were no changes to `src` listings
12. Clean up

Note that for the remote fetches, only git repos are currently supported.
