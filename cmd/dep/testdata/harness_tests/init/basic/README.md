
# `init` Basic Tests

Test project code has one dependency on `sdboyer/deptestdos` (repo "A"), which
in turn has one dependency on `sdboyer/deptest` (repo "B").

Repo A has 1 tag (2.0.0) and three commits (including 5c607 as 2.0.0, and
a0196 after)

Repo B has 3 tags (0.8.0, 0.8.1, and 1.0.0) and multiple commits.


| Test/Passing | Gopath Files    | Vendor Files | Expected Manifest | Expected Lock    | Notes                             |
|--------------|-----------------|--------------|-------------------|------------------|-----------------------------------|
| case1 / Y    | -               | -            | -                 | A 2.0.0, B 1.0.0 |                                   |  
| case2 / N    | A a0196         | -            | A a0196           | A a0196, B 1.0.0 |                                   |  
| case3 / N    | B0.8.0          | -            | -                 | A 2.0.0, B 0.8.0 | Repo B is not a direct dependency |  
| case4 / N    | A a0196, B0.8.0 | -            | A a0196           | A a0196, B 0.8.0 | Repo B is not a direct dependency |  
etc...
