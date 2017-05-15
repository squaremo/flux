package remote

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux/guid"
	"github.com/weaveworks/flux/job"
	"github.com/weaveworks/flux/update"
)

type MockPlatform struct {
	PingError error

	VersionAnswer string
	VersionError  error

	ExportAnswer []byte
	ExportError  error

	ListServicesAnswer []flux.ServiceStatus
	ListServicesError  error

	ListImagesAnswer []flux.ImageStatus
	ListImagesError  error

	UpdateManifestsArgTest func(update.Spec) error
	UpdateManifestsAnswer  job.ID
	UpdateManifestsError   error

	SyncNotifyError error

	SyncStatusAnswer []string
	SyncStatusError  error

	JobStatusAnswer job.Status
	JobStatusError  error
}

func (p *MockPlatform) Ping() error {
	return p.PingError
}

func (p *MockPlatform) Version() (string, error) {
	return p.VersionAnswer, p.VersionError
}

func (p *MockPlatform) Export() ([]byte, error) {
	return p.ExportAnswer, p.ExportError
}

func (p *MockPlatform) ListServices(ns string) ([]flux.ServiceStatus, error) {
	return p.ListServicesAnswer, p.ListServicesError
}

func (p *MockPlatform) ListImages(update.ServiceSpec) ([]flux.ImageStatus, error) {
	return p.ListImagesAnswer, p.ListImagesError
}

func (p *MockPlatform) UpdateManifests(s update.Spec) (job.ID, error) {
	if p.UpdateManifestsArgTest != nil {
		if err := p.UpdateManifestsArgTest(s); err != nil {
			return job.ID(""), err
		}
	}
	return p.UpdateManifestsAnswer, p.UpdateManifestsError
}

func (p *MockPlatform) SyncNotify() error {
	return p.SyncNotifyError
}

func (p *MockPlatform) SyncStatus(string) ([]string, error) {
	return p.SyncStatusAnswer, p.SyncStatusError
}

func (p *MockPlatform) JobStatus(job.ID) (job.Status, error) {
	return p.JobStatusAnswer, p.JobStatusError
}

var _ Platform = &MockPlatform{}

// -- Battery of tests for a platform mechanism. Since these
// essentially wrap the platform in various transports, we expect
// arguments and answers to be preserved.

func PlatformTestBattery(t *testing.T, wrap func(mock Platform) Platform) {
	// set up
	namespace := "the-space-of-names"
	serviceID := flux.ServiceID(namespace + "/service")
	serviceList := []flux.ServiceID{serviceID}
	services := flux.ServiceIDSet{}
	services.Add(serviceList)

	now := time.Now()

	imageID, _ := flux.ParseImageID("quay.io/example.com/frob:v0.4.5")
	serviceAnswer := []flux.ServiceStatus{
		flux.ServiceStatus{
			ID:     flux.ServiceID("foobar/hello"),
			Status: "ok",
			Containers: []flux.Container{
				flux.Container{
					Name: "frobnicator",
					Current: flux.ImageDescription{
						ID:        imageID,
						CreatedAt: &now,
					},
				},
			},
		},
		flux.ServiceStatus{},
	}

	imagesAnswer := []flux.ImageStatus{
		flux.ImageStatus{
			ID:         flux.ServiceID("barfoo/yello"),
			Containers: []flux.Container{},
		},
	}

	syncStatusAnswer := []string{
		"commit 1",
		"commit 2",
		"commit 3",
	}

	updateSpec := update.Spec{
		Type: update.Images,
		Spec: update.ReleaseSpec{
			ServiceSpecs: []update.ServiceSpec{
				update.ServiceSpecAll,
			},
			ImageSpec: update.ImageSpecLatest,
		},
	}
	checkUpdateSpec := func(s update.Spec) error {
		if !reflect.DeepEqual(updateSpec, s) {
			return errors.New("expected != actual")
		}
		return nil
	}

	mock := &MockPlatform{
		ListServicesAnswer:     serviceAnswer,
		ListImagesAnswer:       imagesAnswer,
		UpdateManifestsArgTest: checkUpdateSpec,
		UpdateManifestsAnswer:  job.ID(guid.New()),
		SyncStatusAnswer:       syncStatusAnswer,
	}

	// OK, here we go
	client := wrap(mock)

	if err := client.Ping(); err != nil {
		t.Fatal(err)
	}

	ss, err := client.ListServices(namespace)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(ss, mock.ListServicesAnswer) {
		if diff, err := Diff(ss, mock.ListServicesAnswer); err != nil || len(diff) > 0 {
			t.Error(fmt.Errorf("expected:\n%#v\ngot:\n%#v", mock.ListServicesAnswer, ss))
		} else {
			t.Fatal("DeepEqual says different, Diff says the same!")
		}
	}
	mock.ListServicesError = fmt.Errorf("list services query failure")
	ss, err = client.ListServices(namespace)
	if err == nil {
		t.Error("expected error from ListServices, got nil")
	}

	ims, err := client.ListImages(update.ServiceSpecAll)
	if err != nil {
		t.Error(err)
	}
	if diff, err := Diff(ims, mock.ListImagesAnswer); err != nil || len(diff) > 0 {
		printDiff(diff)
		t.Errorf("expected:\n%#v\ngot:\n%#v", mock.ListImagesAnswer, ims)
	}
	mock.ListImagesError = fmt.Errorf("list images error")
	if _, err = client.ListImages(update.ServiceSpecAll); err == nil {
		t.Error("expected error from ListImages, got nil")
	}

	jobid, err := mock.UpdateManifests(updateSpec)
	if err != nil {
		t.Error(err)
	}
	if jobid != mock.UpdateManifestsAnswer {
		t.Error(fmt.Errorf("expected %q, got %q", mock.UpdateManifestsAnswer, jobid))
	}
	mock.UpdateManifestsError = fmt.Errorf("update manifests error")
	if _, err = client.UpdateManifests(updateSpec); err == nil {
		t.Error("expected error from UpdateManifests, got nil")
	}

	if err := client.SyncNotify(); err != nil {
		t.Error(err)
	}

	syncSt, err := client.SyncStatus("HEAD")
	if err != nil {
		t.Error(err)
	}
	if diff, err := Diff(mock.SyncStatusAnswer, syncSt); err != nil {
		printDiff(diff)
		t.Errorf("expected: %#v\ngot: %#v", mock.SyncStatusAnswer, syncSt)
	}
}

// ===

var ErrNotDiffable = errors.New("values are not diffable")

type Chunk struct {
	Deleted []interface{}
	Added   []interface{}
	Path    string
}

func printDiff(diff []Chunk) {
	for _, d := range diff {
		fmt.Printf("At %s:\n", d.Path)
		for _, del := range d.Deleted {
			fmt.Printf("- #v\n", del)
		}
		for _, add := range d.Added {
			fmt.Printf("+ %#v\n", add)
		}
		println()
	}
}

// Diff one object with another. This assumes that the objects being
// compared are supposed to represent the same logical object, i.e.,
// they were identified with the same ID. An error indicates they are
// not comparable.
func Diff(a, b interface{}) ([]Chunk, error) {
	// Special case at the top: if these have different runtime types,
	// they are not comparable.
	typA, typB := reflect.TypeOf(a), reflect.TypeOf(b)
	if typA != typB {
		return nil, ErrNotDiffable
	}
	return diffValue(reflect.ValueOf(a), reflect.ValueOf(b), typA, "")
}

func Changed(A, B interface{}, path string) Chunk {
	return Chunk{
		Path:    path,
		Deleted: []interface{}{A},
		Added:   []interface{}{B},
	}
}

func Added(B interface{}, path string) Chunk {
	return Chunk{
		Path:  path,
		Added: []interface{}{B},
	}
}

func Removed(A interface{}, path string) Chunk {
	return Chunk{
		Path:    path,
		Deleted: []interface{}{A},
	}
}

// Compare two reflected values and compile a list of differences
// between them.
func diffValue(a, b reflect.Value, typ reflect.Type, path string) ([]Chunk, error) {
	switch typ.Kind() {
	case reflect.Array:
		fallthrough
	case reflect.Slice:
		return diffArrayOrSlice(a, b, typ, path)
	case reflect.Interface:
		return nil, errors.New("interface diff not implemented")
	case reflect.Ptr:
		a, b, typ = reflect.Indirect(a), reflect.Indirect(b), typ.Elem()
		return diffValue(a, b, typ, path)
	case reflect.Struct:
		return diffStruct(a, b, typ, path)
	case reflect.Map:
		return diffMap(a, b, typ.Elem(), path)
	case reflect.Func:
		return nil, errors.New("func diff not implemented (and not implementable)")
	default: // all ground types
		if a.Interface() != b.Interface() {
			return []Chunk{Changed(a.Interface(), b.Interface(), path)}, nil
		}
		return nil, nil
	}
}

// diff each exported field individually. TODO: treat a struct with
// diffs in ground values as a single chunk, rather than always
// recursing.
func diffStruct(a, b reflect.Value, structTyp reflect.Type, path string) ([]Chunk, error) {
	var diffs []Chunk

	for i := 0; i < structTyp.NumField(); i++ {
		field := structTyp.Field(i)
		if field.PkgPath == "" { // i.e., is an exported field
			fieldDiffs, err := diffValue(a.Field(i), b.Field(i), field.Type, path+"."+field.Name)
			if err != nil {
				return nil, err
			}
			diffs = append(diffs, fieldDiffs...)
		}
	}
	return diffs, nil
}

// diff each element, and include over- or underbite. TODO report an
// array of ground values as a single chunk, rather than recursing.
func diffArrayOrSlice(a, b reflect.Value, sliceTyp reflect.Type, path string) ([]Chunk, error) {
	var changed []Chunk
	elemTyp := sliceTyp.Elem()

	i := 0
	for ; i < a.Len() && i < b.Len(); i++ {
		d, err := diffValue(a.Index(i), b.Index(i), elemTyp, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		changed = append(changed, d...)
	}

	if i < a.Len() {
		var deleted []interface{}
		for j := i; j < a.Len(); j++ {
			deleted = append(deleted, a.Index(j).Interface())
		}
		return append(changed, Chunk{Deleted: deleted, Path: fmt.Sprintf("%s[%d]", path, i)}), nil
	}
	if i < b.Len() {
		var added []interface{}
		for j := i; j < b.Len(); j++ {
			added = append(added, b.Index(j).Interface())
		}
		return append(changed, Chunk{Added: added, Path: fmt.Sprintf("%s[%d]", path, i)}), nil
	}
	return changed, nil
}

// diff each entry in the map, and include entries in only one of A,
// B.
func diffMap(a, b reflect.Value, elemTyp reflect.Type, path string) ([]Chunk, error) {
	if a.Kind() != reflect.Map || b.Kind() != reflect.Map {
		return nil, errors.New("both values must be maps")
	}

	var diffs []Chunk
	var zero reflect.Value
	for _, keyA := range a.MapKeys() {
		valA := a.MapIndex(keyA)
		if valB := b.MapIndex(keyA); valB != zero {
			moreDiffs, err := diffValue(valA, valB, elemTyp, fmt.Sprintf(`%s[%v]`, path, keyA))
			if err != nil {
				return nil, err
			}
			diffs = append(diffs, moreDiffs...)
		} else {
			diffs = append(diffs, Removed(valA.Interface(), fmt.Sprintf(`%s[%v]`, path, keyA)))
		}
	}
	for _, keyB := range b.MapKeys() {
		valB := b.MapIndex(keyB)
		if valA := a.MapIndex(keyB); valA == zero {
			diffs = append(diffs, Added(valB.Interface(), fmt.Sprintf(`%s[%v]`, path, keyB)))
		}
	}

	sort.Sort(sorted(diffs))
	return diffs, nil
}

// It helps to return the differences for a map in a stable order
type sorted []Chunk

func (d sorted) Len() int {
	return len(d)
}

// Sort order for chunks: lexically on path
func (d sorted) Less(i, j int) bool {
	return strings.Compare(d[i].Path, d[j].Path) == -1
}

func (d sorted) Swap(a, b int) {
	d[a], d[b] = d[b], d[a]
}
