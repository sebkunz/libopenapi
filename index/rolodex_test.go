// Copyright 2023 Princess B33f Heavy Industries / Dave Shanley
// SPDX-License-Identifier: MIT

package index

import (
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestRolodex_NewRolodex(t *testing.T) {
	c := CreateOpenAPIIndexConfig()
	rolo := NewRolodex(c)
	assert.NotNil(t, rolo)
	assert.NotNil(t, rolo.indexConfig)
	assert.Nil(t, rolo.GetIgnoredCircularReferences())
	assert.Equal(t, rolo.GetIndexingDuration(), time.Duration(0))
	assert.Nil(t, rolo.GetRootIndex())
	assert.Len(t, rolo.GetIndexes(), 0)
	assert.Len(t, rolo.GetCaughtErrors(), 0)
}

func TestRolodex_NoFS(t *testing.T) {

	rolo := NewRolodex(CreateOpenAPIIndexConfig())
	rf, err := rolo.Open("spec.yaml")
	assert.Error(t, err)
	assert.Equal(t, "rolodex has no file systems configured, cannot open 'spec.yaml'. "+
		"Add a BaseURL or BasePath to your configuration so the rolodex knows how to resolve references", err.Error())
	assert.Nil(t, rf)

}

func TestRolodex_LocalNativeFS(t *testing.T) {

	t.Parallel()
	testFS := fstest.MapFS{
		"spec.yaml":             {Data: []byte("hip"), ModTime: time.Now()},
		"subfolder/spec1.json":  {Data: []byte("hop"), ModTime: time.Now()},
		"subfolder2/spec2.yaml": {Data: []byte("chop"), ModTime: time.Now()},
		"subfolder2/hello.jpg":  {Data: []byte("shop"), ModTime: time.Now()},
	}

	baseDir := "/tmp"

	fileFS, err := NewLocalFS(baseDir, testFS)
	if err != nil {
		t.Fatal(err)
	}

	rolo := NewRolodex(CreateOpenAPIIndexConfig())
	rolo.AddLocalFS(baseDir, fileFS)

	f, rerr := rolo.Open("spec.yaml")
	assert.NoError(t, rerr)
	assert.Equal(t, "hip", f.GetContent())

}

func TestRolodex_LocalNonNativeFS(t *testing.T) {

	t.Parallel()
	testFS := fstest.MapFS{
		"spec.yaml":             {Data: []byte("hip"), ModTime: time.Now()},
		"subfolder/spec1.json":  {Data: []byte("hop"), ModTime: time.Now()},
		"subfolder2/spec2.yaml": {Data: []byte("chop"), ModTime: time.Now()},
		"subfolder2/hello.jpg":  {Data: []byte("shop"), ModTime: time.Now()},
	}

	baseDir := ""

	rolo := NewRolodex(CreateOpenAPIIndexConfig())
	rolo.AddLocalFS(baseDir, testFS)

	f, rerr := rolo.Open("spec.yaml")
	assert.NoError(t, rerr)

	assert.Equal(t, "hip", f.GetContent())
}

type test_badfs struct {
	ok       bool
	goodstat bool
	offset   int64
}

func (t *test_badfs) Open(v string) (fs.File, error) {
	ok := false
	if v != "/" && v != "." && v != "http://localhost/test.yaml" {
		ok = true
	}
	if v == "http://localhost/goodstat.yaml" || strings.HasSuffix(v, "goodstat.yaml") {
		ok = true
		t.goodstat = true
	}
	if v == "http://localhost/badstat.yaml" || v == "badstat.yaml" {
		ok = true
		t.goodstat = false
	}
	return &test_badfs{ok: ok, goodstat: t.goodstat}, nil
}
func (t *test_badfs) Stat() (fs.FileInfo, error) {
	if t.goodstat {
		return &LocalFile{
			lastModified: time.Now(),
		}, nil
	}
	return nil, os.ErrInvalid
}
func (t *test_badfs) Read(b []byte) (int, error) {
	if t.ok {
		if t.offset >= int64(len("pizza")) {
			return 0, io.EOF
		}
		if t.offset < 0 {
			return 0, &fs.PathError{Op: "read", Path: "lemons", Err: fs.ErrInvalid}
		}
		n := copy(b, "pizza"[t.offset:])
		t.offset += int64(n)
		return n, nil
	}
	return 0, os.ErrNotExist
}
func (t *test_badfs) Close() error {
	return os.ErrNotExist
}

func TestRolodex_LocalNonNativeFS_BadRead(t *testing.T) {

	t.Parallel()
	testFS := &test_badfs{}

	baseDir := ""

	rolo := NewRolodex(CreateOpenAPIIndexConfig())
	rolo.AddLocalFS(baseDir, testFS)

	f, rerr := rolo.Open("/")
	assert.Nil(t, f)
	assert.Error(t, rerr)
	assert.Equal(t, "file does not exist", rerr.Error())

}

func TestRolodex_LocalNonNativeFS_BadStat(t *testing.T) {

	t.Parallel()
	testFS := &test_badfs{}

	baseDir := ""

	rolo := NewRolodex(CreateOpenAPIIndexConfig())
	rolo.AddLocalFS(baseDir, testFS)

	f, rerr := rolo.Open("badstat.yaml")
	assert.Nil(t, f)
	assert.Error(t, rerr)
	assert.Equal(t, "invalid argument", rerr.Error())

}

func TestRolodex_LocalNonNativeRemoteFS_BadRead(t *testing.T) {

	t.Parallel()
	testFS := &test_badfs{}

	baseDir := ""

	rolo := NewRolodex(CreateOpenAPIIndexConfig())
	rolo.AddRemoteFS(baseDir, testFS)

	f, rerr := rolo.Open("http://localhost/test.yaml")
	assert.Nil(t, f)
	assert.Error(t, rerr)
	assert.Equal(t, "file does not exist", rerr.Error())
}

func TestRolodex_LocalNonNativeRemoteFS_ReadFile(t *testing.T) {

	t.Parallel()
	testFS := &test_badfs{}

	baseDir := ""

	rolo := NewRolodex(CreateOpenAPIIndexConfig())
	rolo.AddRemoteFS(baseDir, testFS)

	r, rerr := rolo.Open("http://localhost/goodstat.yaml")
	assert.NotNil(t, r)
	assert.NoError(t, rerr)

	assert.Equal(t, "goodstat.yaml", r.Name())
	assert.Nil(t, r.GetIndex())
	assert.Equal(t, "pizza", r.GetContent())
	assert.Equal(t, "http://localhost/goodstat.yaml", r.GetFullPath())
	assert.Equal(t, time.Now().UnixMilli(), r.ModTime().UnixMilli())
	assert.Equal(t, int64(5), r.Size())
	assert.False(t, r.IsDir())
	assert.Nil(t, r.Sys())
	assert.Equal(t, r.Mode(), os.FileMode(0))
	n, e := r.GetContentAsYAMLNode()
	assert.Len(t, r.GetErrors(), 0)
	assert.NoError(t, e)
	assert.NotNil(t, n)
	assert.Equal(t, YAML, r.GetFileExtension())
}

func TestRolodex_LocalNonNativeRemoteFS_BadStat(t *testing.T) {

	t.Parallel()
	testFS := &test_badfs{}

	baseDir := ""

	rolo := NewRolodex(CreateOpenAPIIndexConfig())
	rolo.AddRemoteFS(baseDir, testFS)

	f, rerr := rolo.Open("http://localhost/badstat.yaml")
	assert.Nil(t, f)
	assert.Error(t, rerr)
	assert.Equal(t, "invalid argument", rerr.Error())

}

func TestRolodex_rolodexFileTests(t *testing.T) {
	r := &rolodexFile{}
	assert.Equal(t, "", r.Name())
	assert.Nil(t, r.GetIndex())
	assert.Equal(t, "", r.GetContent())
	assert.Equal(t, "", r.GetFullPath())
	assert.Equal(t, time.Now().UnixMilli(), r.ModTime().UnixMilli())
	assert.Equal(t, int64(0), r.Size())
	assert.False(t, r.IsDir())
	assert.Nil(t, r.Sys())
	assert.Equal(t, r.Mode(), os.FileMode(0))
	n, e := r.GetContentAsYAMLNode()
	assert.Len(t, r.GetErrors(), 0)
	assert.NoError(t, e)
	assert.Nil(t, n)
	assert.Equal(t, UNSUPPORTED, r.GetFileExtension())
}

func TestRolodex_NotRolodexFS(t *testing.T) {

	nonRoloFS := os.DirFS(".")
	cf := CreateOpenAPIIndexConfig()
	rolo := NewRolodex(cf)
	rolo.AddLocalFS(".", nonRoloFS)

	err := rolo.IndexTheRolodex()

	assert.Error(t, err)
	assert.Equal(t, "rolodex file system is not a RolodexFS", err.Error())

}

func TestRolodex_IndexCircularLookup(t *testing.T) {

	offToOz := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      $ref: "../test_specs/circular-tests.yaml#/components/schemas/One"`

	_ = os.WriteFile("off_to_oz.yaml", []byte(offToOz), 0644)
	defer os.Remove("off_to_oz.yaml")

	baseDir := "../"

	fsCfg := &LocalFSConfig{
		BaseDirectory: baseDir,
		DirFS:         os.DirFS(baseDir),
		FileFilters: []string{
			"off_to_oz.yaml",
			"test_specs/circular-tests.yaml",
		},
	}

	fileFS, err := NewLocalFSWithConfig(fsCfg)
	if err != nil {
		t.Fatal(err)
	}

	cf := CreateOpenAPIIndexConfig()
	cf.BasePath = baseDir
	rolodex := NewRolodex(cf)
	rolodex.AddLocalFS(baseDir, fileFS)
	err = rolodex.IndexTheRolodex()
	assert.Error(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 3)
	assert.Len(t, rolodex.GetIgnoredCircularReferences(), 0)
}

func TestRolodex_IndexCircularLookup_AroundWeGo(t *testing.T) {

	there := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: object
      required:
        - where
      properties:
        where:
          $ref: "back-again.yaml#/components/schemas/CircleTest/properties/muffins"`

	backagain := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         $ref: "there.yaml#/components/schemas/CircleTest"`

	_ = os.WriteFile("there.yaml", []byte(there), 0644)
	_ = os.WriteFile("back-again.yaml", []byte(backagain), 0644)
	defer os.Remove("there.yaml")
	defer os.Remove("back-again.yaml")

	baseDir := "."

	fsCfg := &LocalFSConfig{
		BaseDirectory: baseDir,
		DirFS:         os.DirFS(baseDir),
		FileFilters: []string{
			"there.yaml",
			"back-again.yaml",
		},
	}

	fileFS, err := NewLocalFSWithConfig(fsCfg)
	if err != nil {
		t.Fatal(err)
	}

	cf := CreateOpenAPIIndexConfig()
	cf.BasePath = baseDir
	rolodex := NewRolodex(cf)
	rolodex.AddLocalFS(baseDir, fileFS)
	err = rolodex.IndexTheRolodex()
	assert.Error(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 1)
	assert.Len(t, rolodex.GetIgnoredCircularReferences(), 0)
}

func TestRolodex_IndexCircularLookup_AroundWeGo_IgnorePoly(t *testing.T) {

	fifth := "type: string"

	fourth := `type: "object"
properties:
  name:
    type: "string"
  children:
    type: "object"`

	third := `type: "object"
properties:
  name:
    $ref: "http://the-space-race-is-all-about-space-and-time-dot.com/fourth.yaml"
  tame: 
    $ref: "http://the-space-race-is-all-about-space-and-time-dot.com/fifth.yaml#/"
  blame:
    $ref: "fifth.yaml"

  fame: 
     $ref: "$PWD/fourth.yaml#/properties/name"
  game:
    $ref: "$PWD/fifth.yaml"

  children:
    type: "object"
    anyOf:
      - $ref: "second.yaml#/components/schemas/CircleTest"
required:
  - children`

	second := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          anyOf:
            - $ref: "third.yaml"
          description: "Array of sub-categories in the same format."
      required:
        - "name"
        - "children"`

	first := `openapi: 3.1.0
components:
  schemas:
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         $ref: "second.yaml#/components/schemas/CircleTest"`

	cwd, _ := os.Getwd()

	_ = os.WriteFile("third.yaml", []byte(strings.ReplaceAll(third, "$PWD", cwd)), 0644)
	_ = os.WriteFile("second.yaml", []byte(second), 0644)
	_ = os.WriteFile("first.yaml", []byte(first), 0644)
	_ = os.WriteFile("fourth.yaml", []byte(fourth), 0644)
	_ = os.WriteFile("fifth.yaml", []byte(fifth), 0644)
	defer os.Remove("first.yaml")
	defer os.Remove("second.yaml")
	defer os.Remove("third.yaml")
	defer os.Remove("fourth.yaml")
	defer os.Remove("fifth.yaml")

	baseDir := "."

	fsCfg := &LocalFSConfig{
		BaseDirectory: baseDir,
		DirFS:         os.DirFS(baseDir),
		FileFilters: []string{
			"first.yaml",
			"second.yaml",
			"third.yaml",
			"fourth.yaml",
			"fifth.yaml",
		},
	}

	fileFS, err := NewLocalFSWithConfig(fsCfg)
	if err != nil {
		t.Fatal(err)
	}

	cf := CreateOpenAPIIndexConfig()
	cf.BasePath = baseDir
	cf.IgnorePolymorphicCircularReferences = true
	cf.SkipDocumentCheck = true
	rolodex := NewRolodex(cf)
	rolodex.AddLocalFS(baseDir, fileFS)

	srv := test_rolodexDeepRefServer([]byte(first), []byte(second),
		[]byte(strings.ReplaceAll(third, "$PWD", cwd)), []byte(fourth), []byte(fifth))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cf.BaseURL = u
	remoteFS, rErr := NewRemoteFSWithConfig(cf)
	assert.NoError(t, rErr)

	rolodex.AddRemoteFS(srv.URL, remoteFS)

	err = rolodex.IndexTheRolodex()
	assert.NoError(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 0)

	// there are two circles. Once when reading the journey from first.yaml, and then a second internal look in second.yaml
	// the index won't find three, because by the time that 'three' has been read, it's already been indexed and the journey
	// discovered.
	assert.Len(t, rolodex.GetIgnoredCircularReferences(), 2)

	// extract a local file
	f, _ := rolodex.Open("first.yaml")
	// index
	x, y := f.(*rolodexFile).Index(cf)
	assert.NotNil(t, x)
	assert.NoError(t, y)

	// re-index
	x, y = f.(*rolodexFile).Index(cf)
	assert.NotNil(t, x)
	assert.NoError(t, y)

	// extract a remote  file
	f, _ = rolodex.Open("http://the-space-race-is-all-about-space-and-time-dot.com/fourth.yaml")

	// index
	x, y = f.(*rolodexFile).Index(cf)
	assert.NotNil(t, x)
	assert.NoError(t, y)

	// re-index
	x, y = f.(*rolodexFile).Index(cf)
	assert.NotNil(t, x)
	assert.NoError(t, y)

	// extract another remote  file
	f, _ = rolodex.Open("http://the-space-race-is-all-about-space-and-time-dot.com/fifth.yaml")

	//change cf to perform document check (which should fail)
	cf.SkipDocumentCheck = false

	// index and fail
	x, y = f.(*rolodexFile).Index(cf)
	assert.Nil(t, x)
	assert.Error(t, y)
}

func test_rolodexDeepRefServer(a, b, c, d, e []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Last-Modified", "Wed, 21 Oct 2015 12:28:00 GMT")
		if strings.HasSuffix(req.URL.String(), "/first.yaml") {
			_, _ = rw.Write(a)
			return
		}
		if strings.HasSuffix(req.URL.String(), "/second.yaml") {
			_, _ = rw.Write(b)
			return
		}
		if strings.HasSuffix(req.URL.String(), "/third.yaml") {
			_, _ = rw.Write(c)
			return
		}
		if strings.HasSuffix(req.URL.String(), "/fourth.yaml") {
			_, _ = rw.Write(d)
			return
		}
		if strings.HasSuffix(req.URL.String(), "/fifth.yaml") {
			_, _ = rw.Write(e)
			return
		}
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte("500 - COMPUTAR SAYS NO!"))
	}))
}

func TestRolodex_IndexCircularLookup_PolyItems_LocalLoop_WithFiles_RecursiveLookup(t *testing.T) {

	fourth := `type: "object"
properties:
  name:
    type: "string"
  children:
    type: "object"`

	third := `type: "object"
properties:
  name:
    $ref: "http://the-space-race-is-all-about-space-and-time-dot.com/fourth.yaml"`

	second := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: "object"
      properties:
        bing:
          $ref: "not_found.yaml"
        name:
          type: "string"
        children:
          type: "object"
          anyOf:
            - $ref: "third.yaml"
      required:
        - "name"
        - "children"`

	first := `openapi: 3.1.0
components:
  schemas:
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         $ref: "second_n.yaml#/components/schemas/CircleTest"`

	cwd, _ := os.Getwd()

	_ = os.WriteFile("third_n.yaml", []byte(strings.ReplaceAll(third, "$PWD", cwd)), 0644)
	_ = os.WriteFile("second_n.yaml", []byte(second), 0644)
	_ = os.WriteFile("first_n.yaml", []byte(first), 0644)
	_ = os.WriteFile("fourth_n.yaml", []byte(fourth), 0644)
	defer os.Remove("first_n.yaml")
	defer os.Remove("second_n.yaml")
	defer os.Remove("third_n.yaml")
	defer os.Remove("fourth_n.yaml")

	baseDir := "."
	cf := CreateOpenAPIIndexConfig()
	cf.BasePath = baseDir
	cf.IgnorePolymorphicCircularReferences = true

	fsCfg := &LocalFSConfig{
		BaseDirectory: baseDir,
		IndexConfig:   cf,
	}

	fileFS, err := NewLocalFSWithConfig(fsCfg)
	if err != nil {
		t.Fatal(err)
	}

	rolodex := NewRolodex(cf)
	rolodex.AddLocalFS(baseDir, fileFS)

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(first), &rootNode)
	rolodex.SetRootNode(&rootNode)

	srv := test_rolodexDeepRefServer([]byte(first), []byte(second),
		[]byte(strings.ReplaceAll(third, "$PWD", cwd)), []byte(fourth), nil)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cf.BaseURL = u
	remoteFS, rErr := NewRemoteFSWithConfig(cf)
	assert.NoError(t, rErr)

	rolodex.AddRemoteFS(srv.URL, remoteFS)

	err = rolodex.IndexTheRolodex()
	assert.Error(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 2)
}

func TestRolodex_IndexCircularLookup_PolyItems_LocalLoop_WithFiles(t *testing.T) {

	first := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          oneOf:
            items:
              $ref: "second_a.yaml#/components/schemas/CircleTest"
      required:
        - "name"
        - "children"
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         type: object
         anyOf:
           - $ref: "#/components/schemas/CircleTest"`

	second := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          oneOf:
            items:
              $ref: "#/components/schemas/CircleTest"
      required:
        - "name"
        - "children"
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         type: object
         anyOf:
           - $ref: "#/components/schemas/CircleTest"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(first), &rootNode)

	_ = os.WriteFile("second_a.yaml", []byte(second), 0644)
	_ = os.WriteFile("first_a.yaml", []byte(first), 0644)
	defer os.Remove("first_a.yaml")
	defer os.Remove("second_a.yaml")

	cf := CreateOpenAPIIndexConfig()
	cf.IgnorePolymorphicCircularReferences = true
	rolodex := NewRolodex(cf)

	baseDir := "."

	fsCfg := &LocalFSConfig{
		BaseDirectory: baseDir,
		DirFS:         os.DirFS(baseDir),
		FileFilters: []string{
			"first_a.yaml",
			"second_a.yaml",
		},
	}

	fileFS, err := NewLocalFSWithConfig(fsCfg)
	if err != nil {
		t.Fatal(err)
	}

	rolodex.AddLocalFS(baseDir, fileFS)
	rolodex.SetRootNode(&rootNode)
	assert.NotNil(t, rolodex.GetRootNode())

	err = rolodex.IndexTheRolodex()
	assert.NoError(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 0)

	// multiple loops across two files
	assert.Len(t, rolodex.GetIgnoredCircularReferences(), 1)
}

func TestRolodex_IndexCircularLookup_PolyItems_LocalLoop_BuildIndexesPost(t *testing.T) {

	first := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          oneOf:
            items:
              $ref: "second_d.yaml#/components/schemas/CircleTest"
      required:
        - "name"
        - "children"
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         type: object
         anyOf:
           - $ref: "#/components/schemas/CircleTest"`

	second := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          oneOf:
            items:
              $ref: "#/components/schemas/CircleTest"
      required:
        - "name"
        - "children"
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         type: object
         anyOf:
           - $ref: "#/components/schemas/CircleTest"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(first), &rootNode)

	_ = os.WriteFile("second_d.yaml", []byte(second), 0644)
	_ = os.WriteFile("first_d.yaml", []byte(first), 0644)
	defer os.Remove("first_d.yaml")
	defer os.Remove("second_d.yaml")

	cf := CreateOpenAPIIndexConfig()
	cf.IgnorePolymorphicCircularReferences = true
	cf.AvoidBuildIndex = true
	rolodex := NewRolodex(cf)

	baseDir := "."

	fsCfg := &LocalFSConfig{
		BaseDirectory: baseDir,
		DirFS:         os.DirFS(baseDir),
		FileFilters: []string{
			"first_d.yaml",
			"second_d.yaml",
		},
	}

	fileFS, err := NewLocalFSWithConfig(fsCfg)
	if err != nil {
		t.Fatal(err)
	}

	rolodex.AddLocalFS(baseDir, fileFS)
	rolodex.SetRootNode(&rootNode)

	err = rolodex.IndexTheRolodex()
	rolodex.BuildIndexes()

	assert.NoError(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 0)

	// multiple loops across two files
	assert.Len(t, rolodex.GetIgnoredCircularReferences(), 1)

	// trigger a rebuild, should do nothing.
	rolodex.BuildIndexes()
	assert.Len(t, rolodex.GetCaughtErrors(), 0)

}

func TestRolodex_IndexCircularLookup_ArrayItems_LocalLoop_WithFiles(t *testing.T) {

	first := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "array"
          items:
            $ref: "second_b.yaml#/components/schemas/CircleTest"
      required:
        - "name"
        - "children"
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         type: array
         items:
           $ref: "#/components/schemas/CircleTest"`

	second := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: array
          items:
            $ref: "#/components/schemas/CircleTest"
      required:
        - "name"
        - "children"
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         type: array
         items:
           $ref: "#/components/schemas/CircleTest"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(first), &rootNode)

	_ = os.WriteFile("second_b.yaml", []byte(second), 0644)
	_ = os.WriteFile("first_b.yaml", []byte(first), 0644)
	defer os.Remove("first_b.yaml")
	defer os.Remove("second_b.yaml")

	cf := CreateOpenAPIIndexConfig()
	cf.IgnoreArrayCircularReferences = true
	rolodex := NewRolodex(cf)

	baseDir := "."

	fsCfg := &LocalFSConfig{
		BaseDirectory: baseDir,
		DirFS:         os.DirFS(baseDir),
		FileFilters: []string{
			"first_b.yaml",
			"second_b.yaml",
		},
	}

	fileFS, err := NewLocalFSWithConfig(fsCfg)
	if err != nil {
		t.Fatal(err)
	}

	rolodex.AddLocalFS(baseDir, fileFS)
	rolodex.SetRootNode(&rootNode)

	err = rolodex.IndexTheRolodex()
	assert.NoError(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 0)

	// multiple loops across two files
	assert.Len(t, rolodex.GetIgnoredCircularReferences(), 1)
}

func TestRolodex_IndexCircularLookup_PolyItemsHttpOnly(t *testing.T) {

	third := `type: string`
	fourth := `components:
  schemas:
    Chicken:
      type: string`

	second := `openapi: 3.1.0
components:
  schemas:
    Loopy:
      type: "object"
      properties:
        cake:
          type: "string"
          anyOf:
            items:
              $ref: "https://I-love-a-good-cake-and-pizza.com/third.yaml"
        pizza:
          type: "string"
          anyOf:
            items:
              $ref: "third.yaml"
        same:
          type: "string"
          oneOf:
            items:
              $ref: "https://kjahsdkjahdkjashdas.com/fourth.yaml#/components/schemas/Chicken"
        name:
          type: "string"
          oneOf:
            items:
              $ref: "https://kjahsdkjahdkjashdas.com/third.yaml#/"
        children:
          type: "object"
          allOf:
            items:
              $ref: "first.yaml#/components/schemas/StartTest"
      required:
        - "name"
        - "children"
    CircleTest:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          oneOf:
            items:
              $ref: "#/components/schemas/Loopy"
      required:
        - "name"
        - "children"`

	first := `openapi: 3.1.0
components:
  schemas:
    StartTest:
      type: object
      required:
        - muffins
      properties:
        chuffins:
          type: object
          allOf: 
            - $ref: "https://kjahsdkjahdkjashdas.com/third.yaml"
        buffins:
          type: object
          allOf: 
            - $ref: "https://kjahsdkjahdkjashdas.com/second.yaml#/"
        muffins:
         type: object
         anyOf:
           - $ref: "https://kjahsdkjahdkjashdas.com/second.yaml#/components/schemas/CircleTest"
`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(first), &rootNode)

	cf := CreateOpenAPIIndexConfig()
	cf.IgnorePolymorphicCircularReferences = true
	rolodex := NewRolodex(cf)

	srv := test_rolodexDeepRefServer([]byte(first), []byte(second), []byte(third), []byte(fourth), nil)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cf.BaseURL = u
	remoteFS, rErr := NewRemoteFSWithConfig(cf)
	assert.NoError(t, rErr)

	rolodex.AddRemoteFS(srv.URL, remoteFS)
	rolodex.SetRootNode(&rootNode)

	err := rolodex.IndexTheRolodex()
	assert.NoError(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 0)

	assert.GreaterOrEqual(t, len(rolodex.GetIgnoredCircularReferences()), 1)
	assert.Equal(t, rolodex.GetRootIndex().GetResolver().GetIndexesVisited(), 6)
}

func TestRolodex_IndexCircularLookup_PolyItemsFileOnly_LocalIncluded(t *testing.T) {

	third := `type: string`

	second := `openapi: 3.1.0
components:
  schemas:
    LoopyMcLoopFace:
      type: "object"
      properties:
        hoop:
          type: object
          allOf:
            items:
              $ref: "third_c.yaml"
        boop:
          type: object
          allOf:
            items:
              $ref: "$PWD/third_c.yaml"
        loop:
          type: object
          oneOf:
            items:
              $ref: "#/components/schemas/LoopyMcLoopFace"
    CircleTest:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          anyOf:
            - $ref: "#/components/schemas/LoopyMcLoopFace"
      required:
        - "name"
        - "children"`

	first := `openapi: 3.1.0
components:
  schemas:
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         type: object
         anyOf:
           - $ref: "second_c.yaml#/components/schemas/CircleTest"
           - $ref: "$PWD/third_c.yaml"`

	var rootNode yaml.Node
	cws, _ := os.Getwd()

	_ = yaml.Unmarshal([]byte(strings.ReplaceAll(first, "$PWD", cws)), &rootNode)
	_ = os.WriteFile("second_c.yaml", []byte(strings.ReplaceAll(second, "$PWD", cws)), 0644)
	_ = os.WriteFile("first_c.yaml", []byte(strings.ReplaceAll(first, "$PWD", cws)), 0644)
	_ = os.WriteFile("third_c.yaml", []byte(third), 0644)

	defer os.Remove("first_c.yaml")
	defer os.Remove("second_c.yaml")
	defer os.Remove("third_c.yaml")

	cf := CreateOpenAPIIndexConfig()
	cf.IgnorePolymorphicCircularReferences = true
	rolodex := NewRolodex(cf)

	baseDir := "."

	fsCfg := &LocalFSConfig{
		BaseDirectory: baseDir,
		DirFS:         os.DirFS(baseDir),
		FileFilters: []string{
			"first_c.yaml",
			"second_c.yaml",
			"third_c.yaml",
		},
	}

	fileFS, err := NewLocalFSWithConfig(fsCfg)
	if err != nil {
		t.Fatal(err)
	}

	rolodex.AddLocalFS(baseDir, fileFS)
	rolodex.SetRootNode(&rootNode)

	err = rolodex.IndexTheRolodex()
	assert.NoError(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 0)

	// should only be a single loop.
	assert.Len(t, rolodex.GetIgnoredCircularReferences(), 1)
}

func TestRolodex_TestDropDownToRemoteFS_CatchErrors(t *testing.T) {

	fourth := `type: "object"
properties:
  name:
    type: "string"
  children:
    type: "object"`

	third := `type: "object"
properties:
  name:
    $ref: "http://the-space-race-is-all-about-space-and-time-dot.com/fourth.yaml"`

	second := `openapi: 3.1.0
components:
  schemas:
    CircleTest:
      type: "object"
      properties:
        bing:
          $ref: "not_found.yaml"
        name:
          type: "string"
        children:
          type: "object"
          anyOf:
            - $ref: "third.yaml"
      required:
        - "name"
        - "children"`

	first := `openapi: 3.1.0
components:
  schemas:
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         $ref: "second_e.yaml#/components/schemas/CircleTest"`

	cwd, _ := os.Getwd()

	_ = os.WriteFile("third_e.yaml", []byte(strings.ReplaceAll(third, "$PWD", cwd)), 0644)
	_ = os.WriteFile("second_e.yaml", []byte(second), 0644)
	_ = os.WriteFile("first_e.yaml", []byte(first), 0644)
	_ = os.WriteFile("fourth_e.yaml", []byte(fourth), 0644)
	defer os.Remove("first_e.yaml")
	defer os.Remove("second_e.yaml")
	defer os.Remove("third_e.yaml")
	defer os.Remove("fourth_e.yaml")

	baseDir := "."

	fsCfg := &LocalFSConfig{
		BaseDirectory: baseDir,
		DirFS:         os.DirFS(baseDir),
		FileFilters: []string{
			"first_e.yaml",
			"second_e.yaml",
			"third_e.yaml",
			"fourth_e.yaml",
		},
	}

	fileFS, err := NewLocalFSWithConfig(fsCfg)
	if err != nil {
		t.Fatal(err)
	}

	cf := CreateOpenAPIIndexConfig()
	cf.BasePath = baseDir
	cf.IgnorePolymorphicCircularReferences = true
	rolodex := NewRolodex(cf)
	rolodex.AddLocalFS(baseDir, fileFS)

	srv := test_rolodexDeepRefServer([]byte(first), []byte(second),
		[]byte(strings.ReplaceAll(third, "$PWD", cwd)), []byte(fourth), nil)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cf.BaseURL = u
	remoteFS, rErr := NewRemoteFSWithConfig(cf)
	assert.NoError(t, rErr)

	rolodex.AddRemoteFS(srv.URL, remoteFS)

	err = rolodex.IndexTheRolodex()
	assert.Error(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 2)
}

func TestRolodex_IndexCircularLookup_LookupHttpNoBaseURL(t *testing.T) {

	first := `openapi: 3.1.0
components:
  schemas:
    StartTest:
      type: object
      required:
        - muffins
      properties:
        muffins:
         type: object
         anyOf:
           - $ref: "https://raw.githubusercontent.com/pb33f/libopenapi/main/test_specs/circular-tests.yaml#/components/schemas/One"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(first), &rootNode)

	cf := CreateOpenAPIIndexConfig()
	cf.IgnorePolymorphicCircularReferences = true
	rolodex := NewRolodex(cf)

	remoteFS, rErr := NewRemoteFSWithConfig(cf)
	assert.NoError(t, rErr)

	rolodex.AddRemoteFS("", remoteFS)
	rolodex.SetRootNode(&rootNode)

	err := rolodex.IndexTheRolodex()
	assert.Error(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 1)
}

func TestRolodex_IndexCircularLookup_ignorePoly(t *testing.T) {

	spinny := `openapi: 3.1.0
components:
  schemas:
    ProductCategory:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          anyOf:
            - $ref: "#/components/schemas/ProductCategory"
          description: "Array of sub-categories in the same format."
      required:
        - "name"
        - "children"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(spinny), &rootNode)

	cf := CreateOpenAPIIndexConfig()
	cf.IgnorePolymorphicCircularReferences = true
	rolodex := NewRolodex(cf)
	rolodex.SetRootNode(&rootNode)
	err := rolodex.IndexTheRolodex()
	assert.NoError(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 0)
	assert.Len(t, rolodex.GetIgnoredCircularReferences(), 1)
}

func TestRolodex_IndexCircularLookup_ignoreArray(t *testing.T) {

	spinny := `openapi: 3.1.0
components:
  schemas:
    ProductCategory:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "array"
          items:
            $ref: "#/components/schemas/ProductCategory"
          description: "Array of sub-categories in the same format."
      required:
        - "name"
        - "children"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(spinny), &rootNode)

	cf := CreateOpenAPIIndexConfig()
	cf.IgnoreArrayCircularReferences = true
	rolodex := NewRolodex(cf)
	rolodex.SetRootNode(&rootNode)
	err := rolodex.IndexTheRolodex()
	assert.NoError(t, err)
	assert.Len(t, rolodex.GetCaughtErrors(), 0)
	assert.Len(t, rolodex.GetIgnoredCircularReferences(), 1)
}

func TestRolodex_SimpleTest_OneDoc(t *testing.T) {

	baseDir := "rolodex_test_data"

	fileFS, err := NewLocalFS(baseDir, os.DirFS(baseDir))
	if err != nil {
		t.Fatal(err)
	}

	cf := CreateOpenAPIIndexConfig()
	cf.BasePath = baseDir
	cf.IgnoreArrayCircularReferences = true
	cf.IgnorePolymorphicCircularReferences = true

	rolo := NewRolodex(cf)
	rolo.AddLocalFS(baseDir, fileFS)

	err = rolo.IndexTheRolodex()

	assert.NotZero(t, rolo.GetIndexingDuration())
	assert.Nil(t, rolo.GetRootIndex())
	assert.Len(t, rolo.GetIndexes(), 9)

	assert.NoError(t, err)
	assert.Len(t, rolo.indexes, 9)

	// open components.yaml
	f, rerr := rolo.Open("components.yaml")
	assert.NoError(t, rerr)
	assert.Equal(t, "components.yaml", f.Name())

	idx, ierr := f.(*rolodexFile).Index(cf)
	assert.NoError(t, ierr)
	assert.NotNil(t, idx)
	assert.Equal(t, YAML, f.GetFileExtension())
	assert.True(t, strings.HasSuffix(f.GetFullPath(), "rolodex_test_data/components.yaml"))
	assert.NotNil(t, f.ModTime())
	assert.Equal(t, int64(283), f.Size())
	assert.False(t, f.IsDir())
	assert.Nil(t, f.Sys())
	assert.Equal(t, fs.FileMode(0), f.Mode())
	assert.Len(t, f.GetErrors(), 0)

	// re-run the index should be a no-op
	assert.NoError(t, rolo.IndexTheRolodex())
	rolo.CheckForCircularReferences()
	assert.Len(t, rolo.GetIgnoredCircularReferences(), 0)

}

func TestRolodex_CircularReferencesPolyIgnored(t *testing.T) {

	var d = `openapi: 3.1.0
components:
  schemas:
    bingo:
       type: object
       properties:
         bango:
           $ref: "#/components/schemas/ProductCategory"
    ProductCategory:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          items:
            anyOf:
              items:
                $ref: "#/components/schemas/ProductCategory"
          description: "Array of sub-categories in the same format."
      required:
        - "name"
        - "children"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(d), &rootNode)

	c := CreateClosedAPIIndexConfig()
	c.IgnorePolymorphicCircularReferences = true
	rolo := NewRolodex(c)
	rolo.SetRootNode(&rootNode)
	_ = rolo.IndexTheRolodex()
	assert.NotNil(t, rolo.GetRootIndex())
	rolo.CheckForCircularReferences()
	assert.Len(t, rolo.GetIgnoredCircularReferences(), 1)
	assert.Len(t, rolo.GetCaughtErrors(), 0)

}

func TestRolodex_CircularReferencesPolyIgnored_PostCheck(t *testing.T) {

	var d = `openapi: 3.1.0
components:
  schemas:
    bingo:
       type: object
       properties:
         bango:
           $ref: "#/components/schemas/ProductCategory"
    ProductCategory:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          items:
            anyOf:
              items:
                $ref: "#/components/schemas/ProductCategory"
          description: "Array of sub-categories in the same format."
      required:
        - "name"
        - "children"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(d), &rootNode)

	c := CreateClosedAPIIndexConfig()
	c.IgnorePolymorphicCircularReferences = true
	c.AvoidCircularReferenceCheck = true
	rolo := NewRolodex(c)
	rolo.SetRootNode(&rootNode)
	_ = rolo.IndexTheRolodex()
	assert.NotNil(t, rolo.GetRootIndex())
	rolo.CheckForCircularReferences()
	assert.Len(t, rolo.GetIgnoredCircularReferences(), 1)
	assert.Len(t, rolo.GetCaughtErrors(), 0)

}

func TestRolodex_CircularReferencesPolyIgnored_Resolve(t *testing.T) {

	var d = `openapi: 3.1.0
components:
  schemas:
    bingo:
       type: object
       properties:
         bango:
           $ref: "#/components/schemas/ProductCategory"
    ProductCategory:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "object"
          items:
            anyOf:
              items:
                $ref: "#/components/schemas/ProductCategory"
          description: "Array of sub-categories in the same format."
      required:
        - "name"
        - "children"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(d), &rootNode)

	c := CreateClosedAPIIndexConfig()
	c.IgnorePolymorphicCircularReferences = true
	c.AvoidCircularReferenceCheck = true
	rolo := NewRolodex(c)
	rolo.SetRootNode(&rootNode)
	_ = rolo.IndexTheRolodex()
	assert.NotNil(t, rolo.GetRootIndex())
	rolo.Resolve()
	assert.Len(t, rolo.GetIgnoredCircularReferences(), 1)
	assert.Len(t, rolo.GetCaughtErrors(), 0)

}

func TestRolodex_CircularReferencesPostCheck(t *testing.T) {

	var d = `openapi: 3.1.0
components:
  schemas:
    bingo:
       type: object
       properties:
         bango:
           $ref: "#/components/schemas/bingo"
       required:
        - bango`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(d), &rootNode)

	c := CreateClosedAPIIndexConfig()
	c.AvoidCircularReferenceCheck = true
	rolo := NewRolodex(c)
	rolo.SetRootNode(&rootNode)
	_ = rolo.IndexTheRolodex()
	assert.NotNil(t, rolo.GetRootIndex())
	rolo.CheckForCircularReferences()
	assert.Len(t, rolo.GetIgnoredCircularReferences(), 0)
	assert.Len(t, rolo.GetCaughtErrors(), 1)
	assert.Len(t, rolo.GetRootIndex().GetResolver().GetInfiniteCircularReferences(), 1)
	assert.Len(t, rolo.GetRootIndex().GetResolver().GetSafeCircularReferences(), 0)

}

func TestRolodex_CircularReferencesArrayIgnored(t *testing.T) {

	var d = `openapi: 3.1.0
components:
  schemas:
    ProductCategory:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "array"
          items:
            $ref: "#/components/schemas/ProductCategory"
          description: "Array of sub-categories in the same format."
      required:
        - "name"
        - "children"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(d), &rootNode)

	c := CreateClosedAPIIndexConfig()
	c.IgnoreArrayCircularReferences = true
	rolo := NewRolodex(c)
	rolo.SetRootNode(&rootNode)
	_ = rolo.IndexTheRolodex()
	rolo.CheckForCircularReferences()
	assert.Len(t, rolo.GetIgnoredCircularReferences(), 1)
	assert.Len(t, rolo.GetCaughtErrors(), 0)

}

func TestRolodex_CircularReferencesArrayIgnored_Resolve(t *testing.T) {

	var d = `openapi: 3.1.0
components:
  schemas:
    ProductCategory:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "array"
          items:
            $ref: "#/components/schemas/ProductCategory"
          description: "Array of sub-categories in the same format."
      required:
        - "name"
        - "children"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(d), &rootNode)

	c := CreateClosedAPIIndexConfig()
	c.IgnoreArrayCircularReferences = true
	rolo := NewRolodex(c)
	rolo.SetRootNode(&rootNode)
	_ = rolo.IndexTheRolodex()
	rolo.Resolve()
	assert.Len(t, rolo.GetIgnoredCircularReferences(), 1)
	assert.Len(t, rolo.GetCaughtErrors(), 0)

}

func TestRolodex_CircularReferencesArrayIgnored_PostCheck(t *testing.T) {

	var d = `openapi: 3.1.0
components:
  schemas:
    ProductCategory:
      type: "object"
      properties:
        name:
          type: "string"
        children:
          type: "array"
          items:
            $ref: "#/components/schemas/ProductCategory"
          description: "Array of sub-categories in the same format."
      required:
        - "name"
        - "children"`

	var rootNode yaml.Node
	_ = yaml.Unmarshal([]byte(d), &rootNode)

	c := CreateClosedAPIIndexConfig()
	c.IgnoreArrayCircularReferences = true
	c.AvoidCircularReferenceCheck = true
	rolo := NewRolodex(c)
	rolo.SetRootNode(&rootNode)
	_ = rolo.IndexTheRolodex()
	rolo.CheckForCircularReferences()
	assert.Len(t, rolo.GetIgnoredCircularReferences(), 1)
	assert.Len(t, rolo.GetCaughtErrors(), 0)

}