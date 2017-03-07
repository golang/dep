# golang/dep Integration Tests

The `dep` integration tests use a consistent directory structure under `testdata` to set up the initial project state, run `dep` commands, then check against an expected final state to see if the test passes.

The directory structure is as follows:

    testdata/
        category1/
            subcategory1/
                case1/
                    testcase.yaml
                    initial/
                        file1.go
                        manifest.json
                        ...
                    final/
                        manifest.json
                        lock.json
                case2/
                ...

The test code itself simply walks down the directory tree, looking for `testcase.yaml` files.  These files can be as many levels down the tree as desired.  The test name will consist of the directory path from `testdata` to the test case directory itself.  In the above example, the test name would be `category1/subcategory1/case1`, and could be singled out with the `-run` option of `go test` (i.e. `go test github.com/golang/dep/cmp/dep -run Integration/category1/subcategory1/case1`).  New tests can be added simply by adding a new directory with the yaml file to the `testdata` tree.  There is no need for code modification - the new test will be included automatically.

The yaml file needs to be accompanied by `initial` and `final` directories. The `initial` is copied verbatim into the test project before the `dep` commands are run, are the `manifest` and `lock` files in `final`, if present, are used to compare against the test project results after the commands.

The `testcase.yaml` file has the following format:

    # test case description

    imports:
        github.com/sdboyer/deptest: 0.8.0
        github.com/sdboyer/deptestuno: a0196baa11ea047dd65037287451d36b861b00ea

    initialVendors:
        github.com/sdboyer/deptestdos: 1.0.1
        github.com/sdboyer/deptesttres: a0196baa11ea047dd65037287451d36b861b00ea

    commands:
      - init
      - ensure -update

    finalVendors:
        - github.com/sdboyer/deptest
        - github.com/sdboyer/deptestdos

All of the categories are optional - if the `imports` list for a test is empty, for example, it can be completely left out.

The test procedure is as follows:

1. Create a temporary directory for the test project environment
2. Create `src/github.com/golang/notexist` as the project
3. Copy the contents of `initial` to the project
4. `go get` the repos and versions in `imports` to the `src` directory
5. `go get` the repos and versions in `initialVendors` to the `vendor` directory
6. Run the commands in `commands` in order on the project
7. Check the resulting files against those in `final`
8. Check the `vendor` directory for the repos listed under `finalVendors`
9. Check that there were no changes to `src` listings
10.  Clean up
