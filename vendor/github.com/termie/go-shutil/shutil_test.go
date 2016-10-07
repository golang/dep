package shutil

import (
  "bytes"
  "io/ioutil"
  "os"
  "testing"
)


func filesMatch(src, dst string) (bool, error) {
  srcContents, err := ioutil.ReadFile(src)
  if err != nil {
    return false, err
  }

  dstContents, err := ioutil.ReadFile(dst)
  if err != nil {
    return false, err
  }

  if bytes.Compare(srcContents, dstContents) != 0 {
    return false, nil
  }
  return true, nil
}


func TestSameFileError(t *testing.T) {
  _, err := Copy("test/testfile", "test/testfile", false)
  _, ok := err.(*SameFileError)
  if !ok {
    t.Error(err)
  }
}


func TestCopyFile(t *testing.T) {
  // clear out existing files if they exist
  os.Remove("test/testfile3")

  err := CopyFile("test/testfile", "test/testfile3", false)
  if err != nil {
    t.Error(err)
    return
  }

  match, err := filesMatch("test/testfile", "test/testfile3")
  if err != nil {
    t.Error(err)
    return
  }
  if !match {
    t.Fail()
    return
  }

  // And again without clearing the files
  err = CopyFile("test/testfile2", "test/testfile3", false)
  if err != nil {
    t.Error(err)
    return
  }

  match2, err := filesMatch("test/testfile2", "test/testfile3")
  if err != nil {
    t.Error(err)
    return
  }

  if !match2 {
    t.Fail()
    return
  }
}


func TestCopy(t *testing.T) {
  // clear out existing files if they exist
  os.Remove("test/testfile3")

  _, err := Copy("test/testfile", "test/testfile3", false)
  if err != nil {
    t.Error(err)
    return
  }

  match, err := filesMatch("test/testfile", "test/testfile3")
  if err != nil {
    t.Error(err)
    return
  }
  if !match {
    t.Fail()
    return
  }

  // And again without clearing the files
  _, err = Copy("test/testfile2", "test/testfile3", false)
  if err != nil {
    t.Error(err)
    return
  }

  match2, err := filesMatch("test/testfile2", "test/testfile3")
  if err != nil {
    t.Error(err)
    return
  }

  if !match2 {
    t.Fail()
    return
  }
}


func TestCopyTree(t *testing.T) {
  // clear out existing files if they exist
  os.RemoveAll("test/testdir3")

  err := CopyTree("test/testdir", "test/testdir3", nil)
  if err != nil {
    t.Error(err)
    return
  }

  match, err := filesMatch("test/testdir/file1", "test/testdir3/file1")
  if err != nil {
    t.Error(err)
    return
  }
  if !match {
    t.Fail()
    return
  }

  // // And again without clearing the files
  // _, err = Copy("test/testfile2", "test/testfile3", false)
  // if err != nil {
  //   t.Error(err)
  //   return
  // }

  // match2, err := filesMatch("test/testfile2", "test/testfile3")
  // if err != nil {
  //   t.Error(err)
  //   return
  // }

  // if !match2 {
  //   t.Fail()
  //   return
  // }
}

