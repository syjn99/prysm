package mvslice

import (
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

type testObject struct {
	id uint64
}

func (o *testObject) Id() uint64 {
	return o.id
}

func (o *testObject) SetId(id uint64) {
	o.id = id
}

func TestLen(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{1, 2, 3})
	s.cachedLengths[1] = 123
	t.Run("cached", func(t *testing.T) {
		assert.Equal(t, 123, s.Len(&testObject{id: 1}))
	})
	t.Run("not cached", func(t *testing.T) {
		assert.Equal(t, 3, s.Len(&testObject{id: 999}))
	})
}

func TestCopy(t *testing.T) {
	// What we want to check:
	// - shared value is copied
	// - when the source object has an individual value, it is copied
	// - when the source object does not have an individual value, the shared value is copied
	// - when the source object has an appended value, it is copied
	// - when the source object does not have an appended value, nothing is copied
	// - length of destination object is cached

	s := setup()
	src := &testObject{id: 1}
	dst := &testObject{id: 999}

	s.Copy(src, dst)

	assert.Equal(t, (*MultiValueItem[int])(nil), s.individualItems[0])
	assertIndividualFound(t, s, dst.id, 1, 1)
	assertIndividualFound(t, s, dst.id, 2, 3)
	assertIndividualFound(t, s, dst.id, 3, 1)
	assertIndividualNotFound(t, s, dst.id, 4)
	assertAppendedFound(t, s, dst.id, 0, 1)
	assertAppendedFound(t, s, dst.id, 1, 3)
	assertAppendedNotFound(t, s, dst.id, 2)
	l, ok := s.cachedLengths[999]
	require.Equal(t, true, ok)
	assert.Equal(t, 7, l)
}

func TestValue(t *testing.T) {
	t.Run("individual and appended items", func(t *testing.T) {
		s := setup()
		assert.DeepEqual(t, []int{123, 1, 3, 1, 123, 1, 3}, s.Value(&testObject{id: 1}))
		assert.DeepEqual(t, []int{123, 2, 3, 123, 2, 2, 3, 2}, s.Value(&testObject{id: 2}))
	})
	t.Run("empty overlay", func(t *testing.T) {
		s := &Slice[int]{}
		s.Init([]int{1, 2, 3, 4, 5, 6, 7, 8})
		assert.DeepEqual(t, []int{1, 2, 3, 4, 5, 6, 7, 8}, s.Value(&testObject{id: 999}))
	})
	t.Run("overlay of other objects only", func(t *testing.T) {
		s := setup()
		assert.DeepEqual(t, []int{123, 123, 123, 123, 123}, s.Value(&testObject{id: 999}))
	})
	t.Run("sparse individual items only", func(t *testing.T) {
		s := &Slice[int]{}
		s.Init([]int{1, 2, 3, 4, 5, 6, 7, 8})
		obj := &testObject{id: 1}
		require.NoError(t, s.UpdateAt(obj, 6, 70))
		assert.DeepEqual(t, []int{1, 2, 3, 4, 5, 6, 70, 8}, s.Value(obj))
	})
	t.Run("dense individual items only", func(t *testing.T) {
		s := &Slice[int]{}
		s.Init([]int{1, 2, 3, 4, 5, 6, 7, 8})
		obj := &testObject{id: 1}
		for i := range 8 {
			require.NoError(t, s.UpdateAt(obj, uint64(i), 10*(i+1)))
		}
		assert.DeepEqual(t, []int{10, 20, 30, 40, 50, 60, 70, 80}, s.Value(obj))
	})
	t.Run("appended items only", func(t *testing.T) {
		s := &Slice[int]{}
		s.Init([]int{1, 2, 3})
		obj := &testObject{id: 1}
		s.Append(obj, 4)
		s.Append(obj, 5)
		assert.DeepEqual(t, []int{1, 2, 3, 4, 5}, s.Value(obj))
	})
	t.Run("result is owned by the caller", func(t *testing.T) {
		s := setup()
		obj := &testObject{id: 1}
		v := s.Value(obj)
		for i := range v {
			v[i] = -1
		}
		assert.DeepEqual(t, []int{123, 1, 3, 1, 123, 1, 3}, s.Value(obj))
		v = s.Value(&testObject{id: 999})
		v[0] = -1
		assert.DeepEqual(t, []int{123, 123, 123, 123, 123}, s.Value(&testObject{id: 999}))
	})
	t.Run("concurrent reads and writes", func(t *testing.T) {
		// 64 shared items keep the individual items map sparse (at most 12 entries).
		shared := make([]int, 64)
		s := &Slice[int]{}
		s.Init(slices.Clone(shared))
		readers := make([]testObject, 8)
		wants := make([][]int, len(readers))
		for i := range readers {
			readers[i].id = uint64(i + 1)
			require.NoError(t, s.UpdateAt(&readers[i], uint64(i), 100+i))
			s.Append(&readers[i], i)
			wants[i] = slices.Clone(shared)
			wants[i][i] = 100 + i
			wants[i] = append(wants[i], i)
		}
		writer := &testObject{id: 100}
		var wg sync.WaitGroup
		wg.Go(func() {
			for j := range 500 {
				assert.NoError(t, s.UpdateAt(writer, uint64(60+j%4), j))
				s.Append(writer, j)
			}
		})
		for i := range readers {
			wg.Go(func() {
				for range 500 {
					assert.DeepEqual(t, wants[i], s.Value(&readers[i]))
				}
			})
		}
		wg.Wait()
	})
}

// FuzzValue checks Value against a model of each object's slice and against the per-index At path.
// Each three input bytes are one operation: kind, object and argument.
func FuzzValue(f *testing.F) {
	// Few updates: the individual items map stays sparse.
	f.Add([]byte{2, 0, 1, 2, 1, 2, 3, 0, 5, 1, 2, 0, 3, 2, 7, 2, 2, 0x13})
	// Many updates: the individual items map becomes dense, then a detach and a copy shrink it.
	f.Add([]byte{2, 0, 1, 2, 0, 2, 2, 0, 3, 2, 1, 5, 2, 1, 6, 2, 2, 0x31, 3, 1, 4, 0, 0, 0, 1, 3, 2, 2, 3, 0x47, 3, 3, 9})
	f.Fuzz(func(t *testing.T, ops []byte) {
		// With 16 shared items, Value walks the individual items map while it has at most 3 entries
		// and looks up every index after that.
		shared := slices.Repeat([]int{0, 10, 20, 30}, 4)
		s := &Slice[int]{}
		s.Init(slices.Clone(shared))
		objects := make([]testObject, 4)
		want := make([][]int, len(objects))
		for i := range objects {
			objects[i].id = uint64(i + 1)
			want[i] = slices.Clone(shared)
		}
		for ; len(ops) >= 3; ops = ops[3:] {
			i, arg := int(ops[1])%len(objects), int(ops[2])
			switch ops[0] % 4 {
			case 0:
				s.Detach(&objects[i])
				want[i] = slices.Clone(shared)
			case 1:
				j := arg % len(objects)
				if j == i {
					j = (i + 1) % len(objects)
				}
				s.Detach(&objects[i])
				s.Copy(&objects[j], &objects[i])
				want[i] = slices.Clone(want[j])
			case 2:
				// Values collide with the shared items, so updates also revert to them.
				index, val := arg%len(want[i]), (arg>>4)%4*10
				require.NoError(t, s.UpdateAt(&objects[i], uint64(index), val))
				want[i][index] = val
			default:
				s.Append(&objects[i], arg%4)
				want[i] = append(want[i], arg%4)
			}
			for j := range objects {
				v := s.Value(&objects[j])
				require.DeepEqual(t, want[j], v)
				require.Equal(t, len(v), s.Len(&objects[j]))
				for k := range v {
					at, err := s.At(&objects[j], uint64(k))
					require.NoError(t, err)
					require.Equal(t, v[k], at)
				}
				require.DeepEqual(t, want[j], s.Reset(&objects[j]).Value(&testObject{id: 999}))
			}
		}
	})
}

func TestAt(t *testing.T) {
	// What we want to check:
	// - correct values are returned for first object
	// - correct values are returned for second object
	// - ERROR when index too large in general
	// - ERROR when index not too large in general, but too large for an object

	s := setup()
	first := &testObject{id: 1}
	second := &testObject{id: 2}

	v, err := s.At(first, 0)
	require.NoError(t, err)
	assert.Equal(t, 123, v)
	v, err = s.At(first, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, v)
	v, err = s.At(first, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, v)
	v, err = s.At(first, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, v)
	v, err = s.At(first, 4)
	require.NoError(t, err)
	assert.Equal(t, 123, v)
	v, err = s.At(first, 5)
	require.NoError(t, err)
	assert.Equal(t, 1, v)
	v, err = s.At(first, 6)
	require.NoError(t, err)
	assert.Equal(t, 3, v)
	_, err = s.At(first, 7)
	assert.ErrorContains(t, "index 7 out of bounds", err)

	v, err = s.At(second, 0)
	require.NoError(t, err)
	assert.Equal(t, 123, v)
	v, err = s.At(second, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, v)
	v, err = s.At(second, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, v)
	v, err = s.At(second, 3)
	require.NoError(t, err)
	assert.Equal(t, 123, v)
	v, err = s.At(second, 4)
	require.NoError(t, err)
	assert.Equal(t, 2, v)
	v, err = s.At(second, 5)
	require.NoError(t, err)
	assert.Equal(t, 2, v)
	v, err = s.At(second, 6)
	require.NoError(t, err)
	assert.Equal(t, 3, v)
	v, err = s.At(second, 7)
	require.NoError(t, err)
	assert.Equal(t, 2, v)
	_, err = s.At(second, 8)
	assert.ErrorContains(t, "index 8 out of bounds", err)
}

func TestUpdateAt(t *testing.T) {
	// What we want to check:
	// - shared value is updated only for the updated object, creating a new individual value (shared value remains the same)
	// - individual value (different for both objects) is updated to a third value
	// - individual value (different for both objects) is updated to the other object's value
	// - individual value (equal for both objects) is updated
	// - individual value existing only for the updated object is updated
	// - individual value existing only for the other-object appends an item to the individual value
	// - individual value updated to the original shared value removes that individual value
	// - appended value (different for both objects) is updated to a third value
	// - appended value (different for both objects) is updated to the other object's value
	// - appended value (equal for both objects) is updated
	// - appended value existing for one object is updated
	// - ERROR when index too large in general
	// - ERROR when index not too large in general, but too large for an object

	s := setup()
	first := &testObject{id: 1}
	second := &testObject{id: 2}

	require.NoError(t, s.UpdateAt(first, 0, 999))
	assert.Equal(t, 123, s.sharedItems[0])
	assertIndividualFound(t, s, first.id, 0, 999)
	assertIndividualNotFound(t, s, second.id, 0)

	require.NoError(t, s.UpdateAt(first, 1, 999))
	assertIndividualFound(t, s, first.id, 1, 999)
	assertIndividualFound(t, s, second.id, 1, 2)

	require.NoError(t, s.UpdateAt(first, 1, 2))
	assertIndividualFound(t, s, first.id, 1, 2)
	assertIndividualFound(t, s, second.id, 1, 2)

	require.NoError(t, s.UpdateAt(first, 2, 999))
	assertIndividualFound(t, s, first.id, 2, 999)
	assertIndividualFound(t, s, second.id, 2, 3)

	require.NoError(t, s.UpdateAt(first, 3, 999))
	assertIndividualFound(t, s, first.id, 3, 999)
	assertIndividualNotFound(t, s, second.id, 3)

	require.NoError(t, s.UpdateAt(first, 4, 999))
	assertIndividualFound(t, s, first.id, 4, 999)
	assertIndividualFound(t, s, second.id, 4, 2)

	require.NoError(t, s.UpdateAt(first, 4, 123))
	assertIndividualNotFound(t, s, first.id, 4)
	assertIndividualFound(t, s, second.id, 4, 2)

	require.NoError(t, s.UpdateAt(first, 5, 999))
	assertAppendedFound(t, s, first.id, 0, 999)
	assertAppendedFound(t, s, second.id, 0, 2)

	require.NoError(t, s.UpdateAt(first, 5, 2))
	assertAppendedFound(t, s, first.id, 0, 2)
	assertAppendedFound(t, s, second.id, 0, 2)

	require.NoError(t, s.UpdateAt(first, 6, 999))
	assertAppendedFound(t, s, first.id, 1, 999)
	assertAppendedFound(t, s, second.id, 1, 3)

	// we update the second object because there are no more appended items for the first object
	require.NoError(t, s.UpdateAt(second, 7, 999))
	assertAppendedNotFound(t, s, first.id, 2)
	assertAppendedFound(t, s, second.id, 2, 999)

	assert.ErrorContains(t, "index 7 out of bounds", s.UpdateAt(first, 7, 999))
	assert.ErrorContains(t, "index 8 out of bounds", s.UpdateAt(second, 8, 999))
}

func TestAppend(t *testing.T) {
	// What we want to check:
	// - appending first item ever to the slice
	// - appending an item to an object when there is no corresponding item for the other object
	// - appending an item to an object when there is a corresponding item with same value for the other object
	// - appending an item to an object when there is a corresponding item with different value for the other object
	// - we also want to check that cached length is properly updated after every append

	// we want to start with the simplest slice possible
	s := &Slice[int]{}
	s.Init([]int{0})
	first := &testObject{id: 1}
	second := &testObject{id: 2}

	// append first value ever
	s.Append(first, 1)
	require.Equal(t, 1, len(s.appendedItems))
	assertAppendedFound(t, s, first.id, 0, 1)
	assertAppendedNotFound(t, s, second.id, 0)
	l, ok := s.cachedLengths[first.id]
	require.Equal(t, true, ok)
	assert.Equal(t, 2, l)
	_, ok = s.cachedLengths[second.id]
	assert.Equal(t, false, ok)

	// append one more value to the first object, so that we can test two append scenarios for the second object
	s.Append(first, 1)

	// append the first value to the second object, equal to the value for the first object
	s.Append(second, 1)
	require.Equal(t, 2, len(s.appendedItems))
	assertAppendedFound(t, s, first.id, 0, 1)
	assertAppendedFound(t, s, second.id, 0, 1)
	l, ok = s.cachedLengths[first.id]
	require.Equal(t, true, ok)
	assert.Equal(t, 3, l)
	l, ok = s.cachedLengths[second.id]
	assert.Equal(t, true, ok)
	assert.Equal(t, 2, l)

	// append the first value to the second object, different than the value for the first object
	s.Append(second, 2)
	require.Equal(t, 2, len(s.appendedItems))
	assertAppendedFound(t, s, first.id, 1, 1)
	assertAppendedFound(t, s, second.id, 1, 2)
	l, ok = s.cachedLengths[first.id]
	require.Equal(t, true, ok)
	assert.Equal(t, 3, l)
	l, ok = s.cachedLengths[second.id]
	assert.Equal(t, true, ok)
	assert.Equal(t, 3, l)
}

func TestAppendAfterCopyAndDetach(t *testing.T) {
	for _, shared := range [][]int{nil, {10, 20}} {
		t.Run(fmt.Sprintf("shared=%d", len(shared)), func(t *testing.T) {
			s := &Slice[int]{}
			s.Init(shared)
			a, b, c := &testObject{id: 1}, &testObject{id: 2}, &testObject{id: 3}
			s.Append(a, 30)
			s.Copy(a, b)
			s.Append(a, 40)
			s.Append(a, 50)
			s.Append(b, 40)
			s.Append(b, 60)
			s.Copy(b, c)
			s.Detach(b)
			s.Append(c, 70)
			s.Append(b, 80)
			require.DeepEqual(t, append(append([]int{}, shared...), 30, 40, 50), s.Value(a))
			require.DeepEqual(t, append(append([]int{}, shared...), 80), s.Value(b))
			require.DeepEqual(t, append(append([]int{}, shared...), 30, 40, 60, 70), s.Value(c))
			s.Detach(a)
			s.Detach(b)
			s.Detach(c)
			// Reuse empty appended positions left by detached objects.
			for i := range 8 {
				s.Append(a, i)
				require.Equal(t, len(shared)+i+1, s.Len(a))
				got, err := s.At(a, uint64(len(shared)+i))
				require.NoError(t, err)
				require.Equal(t, i, got)
			}
		})
	}
}

func TestAppendModel(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{10, 20})
	objects := make([]testObject, 8)
	want := make([][]int, len(objects))
	for i := range objects {
		objects[i].id = uint64(i + 1)
		want[i] = []int{10, 20}
	}
	rng := rand.New(rand.NewSource(42))
	for step := range 2000 {
		i := rng.Intn(len(objects))
		switch rng.Intn(5) {
		case 0:
			s.Detach(&objects[i])
			want[i] = []int{10, 20}
		case 1:
			j := (i + 1 + rng.Intn(len(objects)-1)) % len(objects)
			s.Detach(&objects[i])
			s.Copy(&objects[j], &objects[i])
			want[i] = append([]int{}, want[j]...)
		case 2:
			index := rng.Intn(len(want[i]))
			require.NoError(t, s.UpdateAt(&objects[i], uint64(index), step))
			want[i][index] = step
		default:
			value := rng.Intn(4)
			s.Append(&objects[i], value)
			want[i] = append(want[i], value)
		}
		for j := range objects {
			require.Equal(t, len(want[j]), s.Len(&objects[j]))
			require.DeepEqual(t, want[j], s.Value(&objects[j]))
		}
	}
}

func TestAppendConcurrentCopies(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{10, 20})
	original := &testObject{id: 0}
	s.Append(original, 30)
	objects := make([]testObject, 8)
	var wg sync.WaitGroup
	for i := range objects {
		objects[i].id = uint64(i + 1)
		s.Copy(original, &objects[i])
		wg.Go(func() {
			for j := range 512 {
				s.Append(&objects[i], j+int(objects[i].id)%2)
			}
		})
	}
	wg.Wait()
	require.DeepEqual(t, []int{10, 20, 30}, s.Value(original))
	for i := range objects {
		want := []int{10, 20, 30}
		for j := range 512 {
			want = append(want, j+int(objects[i].id)%2)
		}
		require.Equal(t, len(want), s.Len(&objects[i]))
		require.DeepEqual(t, want, s.Value(&objects[i]))
	}
}

func BenchmarkAppendScaling(b *testing.B) {
	for _, count := range []int{1000, 10000, 50000} {
		b.Run(fmt.Sprintf("n=%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				s := &Slice[[121]byte]{}
				s.Init(nil)
				obj := &testObject{id: 1}
				for i := range count {
					var value [121]byte
					value[0] = byte(i)
					s.Append(obj, value)
				}
				if s.Len(obj) != count {
					b.Fatal("incorrect appended length")
				}
			}
		})
	}
}

func TestDetach(t *testing.T) {
	// What we want to check:
	// - no individual or appended items left after detaching an object
	// - length removed from cache

	s := setup()
	obj := &testObject{id: 1}

	s.Detach(obj)

	for _, item := range s.individualItems {
		found := false
		for _, v := range item.Values {
			for _, o := range v.ids {
				if o == obj.id {
					found = true
				}
			}
		}
		assert.Equal(t, false, found)
	}
	for _, item := range s.appendedItems {
		found := false
		for _, v := range item.Values {
			for _, o := range v.ids {
				if o == obj.id {
					found = true
				}
			}
		}
		assert.Equal(t, false, found)
	}
	_, ok := s.cachedLengths[obj.id]
	assert.Equal(t, false, ok)
}

func TestReset(t *testing.T) {
	s := setup()
	obj := &testObject{id: 2}

	reset := s.Reset(obj)

	assert.Equal(t, 8, len(reset.sharedItems))
	assert.Equal(t, 123, reset.sharedItems[0])
	assert.Equal(t, 2, reset.sharedItems[1])
	assert.Equal(t, 3, reset.sharedItems[2])
	assert.Equal(t, 123, reset.sharedItems[3])
	assert.Equal(t, 2, reset.sharedItems[4])
	assert.Equal(t, 2, reset.sharedItems[5])
	assert.Equal(t, 3, reset.sharedItems[6])
	assert.Equal(t, 2, reset.sharedItems[7])
	assert.Equal(t, 0, len(reset.individualItems))
	assert.Equal(t, 0, len(reset.appendedItems))
}

func TestFragmentation_IndividualReferences(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{123, 123, 123, 123, 123})
	s.individualItems[1] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 1,
				ids: []uint64{1},
			},
			{
				val: 2,
				ids: []uint64{2},
			},
		},
	}
	s.individualItems[2] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 3,
				ids: []uint64{1, 2},
			},
		},
	}

	numOfRefs := fragmentationLimit / 2
	for i := 3; i < numOfRefs; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}

	assert.Equal(t, false, s.IsFragmented())

	// Add more references to hit fragmentation limit. Id 1
	// has 2 references above.
	for i := numOfRefs; i < numOfRefs+3; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}
	assert.Equal(t, true, s.IsFragmented())
}

func TestFragmentation_AppendedReferences(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{123, 123, 123, 123, 123})
	s.appendedItems = []*MultiValueItem[int]{
		{
			Values: []*Value[int]{
				{
					val: 1,
					ids: []uint64{1},
				},
				{
					val: 2,
					ids: []uint64{2},
				},
			},
		},
		{
			Values: []*Value[int]{
				{
					val: 3,
					ids: []uint64{1, 2},
				},
			},
		},
	}
	s.cachedLengths[1] = 7
	s.cachedLengths[2] = 8

	numOfRefs := fragmentationLimit / 2
	for i := 3; i < numOfRefs; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}

	assert.Equal(t, false, s.IsFragmented())

	// Add more references to hit fragmentation limit. Id 1
	// has 2 references above.
	for i := numOfRefs; i < numOfRefs+3; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}
	assert.Equal(t, true, s.IsFragmented())
}

func TestFragmentation_IndividualAndAppendedReferences(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{123, 123, 123, 123, 123})
	s.individualItems[2] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 3,
				ids: []uint64{1, 2},
			},
		},
	}
	s.appendedItems = []*MultiValueItem[int]{
		{
			Values: []*Value[int]{
				{
					val: 1,
					ids: []uint64{1},
				},
				{
					val: 2,
					ids: []uint64{2},
				},
			},
		},
	}
	s.cachedLengths[1] = 7
	s.cachedLengths[2] = 8

	numOfRefs := fragmentationLimit / 2
	for i := 3; i < numOfRefs; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}

	assert.Equal(t, false, s.IsFragmented())

	// Add more references to hit fragmentation limit. Id 1
	// has 2 references above.
	for i := numOfRefs; i < numOfRefs+3; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}
	assert.Equal(t, true, s.IsFragmented())
}

func TestNil(t *testing.T) {
	obj := &testObject{}

	s := &Slice[int]{}
	s.Init(nil)
	assert.Equal(t, 0, s.Len(obj))
	assert.DeepEqual(t, []int{}, s.Value(obj))
	_, err := s.At(obj, 0)
	assert.NotNil(t, err)
	s.Append(obj, 1)
	assert.Equal(t, 1, s.Len(obj))
	assert.DeepEqual(t, []int{1}, s.Value(obj))
}

// Share the slice between 2 objects.
// Index 0: Shared value
// Index 1: Different individual value
// Index 2: Same individual value
// Index 3: Individual value ONLY for the first object
// Index 4: Individual value ONLY for the second object
// Index 5: Different appended value
// Index 6: Same appended value
// Index 7: Appended value ONLY for the second object
func setup() *Slice[int] {
	s := &Slice[int]{}
	s.Init([]int{123, 123, 123, 123, 123})
	s.individualItems[1] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 1,
				ids: []uint64{1},
			},
			{
				val: 2,
				ids: []uint64{2},
			},
		},
	}
	s.individualItems[2] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 3,
				ids: []uint64{1, 2},
			},
		},
	}
	s.individualItems[3] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 1,
				ids: []uint64{1},
			},
		},
	}
	s.individualItems[4] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 2,
				ids: []uint64{2},
			},
		},
	}
	s.appendedItems = []*MultiValueItem[int]{
		{
			Values: []*Value[int]{
				{
					val: 1,
					ids: []uint64{1},
				},
				{
					val: 2,
					ids: []uint64{2},
				},
			},
		},
		{
			Values: []*Value[int]{
				{
					val: 3,
					ids: []uint64{1, 2},
				},
			},
		},
		{
			Values: []*Value[int]{
				{
					val: 2,
					ids: []uint64{2},
				},
			},
		},
	}
	s.cachedLengths[1] = 7
	s.cachedLengths[2] = 8

	return s
}

func assertIndividualFound(t *testing.T, slice *Slice[int], id uint64, itemIndex uint64, expected int) {
	found := false
	for _, v := range slice.individualItems[itemIndex].Values {
		for _, o := range v.ids {
			if o == id {
				found = true
				assert.Equal(t, expected, v.val)
			}
		}
	}
	assert.Equal(t, true, found)
}

func assertIndividualNotFound(t *testing.T, slice *Slice[int], id uint64, itemIndex uint64) {
	found := false
	for _, v := range slice.individualItems[itemIndex].Values {
		for _, o := range v.ids {
			if o == id {
				found = true
			}
		}
	}
	assert.Equal(t, false, found)
}

func assertAppendedFound(t *testing.T, slice *Slice[int], id uint64, itemIndex uint64, expected int) {
	found := false
	for _, v := range slice.appendedItems[itemIndex].Values {
		for _, o := range v.ids {
			if o == id {
				found = true
				assert.Equal(t, expected, v.val)
			}
		}
	}
	assert.Equal(t, true, found)
}

func assertAppendedNotFound(t *testing.T, slice *Slice[int], id uint64, itemIndex uint64) {
	found := false
	for _, v := range slice.appendedItems[itemIndex].Values {
		for _, o := range v.ids {
			if o == id {
				found = true
			}
		}
	}
	assert.Equal(t, false, found)
}

func BenchmarkValue(b *testing.B) {
	const _100k = 100000
	const _1m = 1000000
	const _10m = 10000000

	b.Run("100,000 shared items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k))
		for b.Loop() {
			s.Value(&testObject{})
		}
	})
	b.Run("100,000 equal individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k))
		s.individualItems[0] = &MultiValueItem[int]{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}
		objs := make([]*testObject, _100k)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[0].Values[0].ids = append(s.individualItems[0].Values[0].ids, uint64(i))
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_100k)])
		}
	})
	b.Run("100,000 different individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k))
		objs := make([]*testObject, _100k)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[uint64(i)] = &MultiValueItem[int]{Values: []*Value[int]{{val: i, ids: []uint64{uint64(i)}}}}
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_100k)])
		}
	})
	b.Run("100,000 shared items and 100,000 equal appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k))
		s.appendedItems = []*MultiValueItem[int]{{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}}
		objs := make([]*testObject, _100k)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems[0].Values[0].ids = append(s.appendedItems[0].Values[0].ids, uint64(i))
			s.cachedLengths[uint64(i)] = _100k + 1
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_100k)])
		}
	})
	b.Run("100,000 shared items and 100,000 different appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k))
		s.appendedItems = []*MultiValueItem[int]{{}}
		objs := make([]*testObject, _100k)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems[0].Values = append(s.appendedItems[0].Values, &Value[int]{val: i, ids: []uint64{uint64(i)}})
			s.cachedLengths[uint64(i)] = _100k + 1
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_100k)])
		}
	})
	b.Run("1,000,000 shared items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m))
		for b.Loop() {
			s.Value(&testObject{})
		}
	})
	b.Run("1,000,000 equal individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m))
		s.individualItems[0] = &MultiValueItem[int]{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}
		objs := make([]*testObject, _1m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[0].Values[0].ids = append(s.individualItems[0].Values[0].ids, uint64(i))
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_1m)])
		}
	})
	b.Run("1,000,000 different individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m))
		objs := make([]*testObject, _1m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[uint64(i)] = &MultiValueItem[int]{Values: []*Value[int]{{val: i, ids: []uint64{uint64(i)}}}}
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_1m)])
		}
	})
	b.Run("1,000,000 shared items and 1,000,000 equal appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m))
		s.appendedItems = []*MultiValueItem[int]{{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}}
		objs := make([]*testObject, _1m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems[0].Values[0].ids = append(s.appendedItems[0].Values[0].ids, uint64(i))
			s.cachedLengths[uint64(i)] = _1m + 1
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_1m)])
		}
	})
	b.Run("1,000,000 shared items and 1,000,000 different appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m))
		s.appendedItems = []*MultiValueItem[int]{{}}
		objs := make([]*testObject, _1m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems[0].Values = append(s.appendedItems[0].Values, &Value[int]{val: i, ids: []uint64{uint64(i)}})
			s.cachedLengths[uint64(i)] = _1m + 1
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_1m)])
		}
	})
	b.Run("10,000,000 shared items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m))
		for b.Loop() {
			s.Value(&testObject{})
		}
	})
	b.Run("10,000,000 equal individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m))
		s.individualItems[0] = &MultiValueItem[int]{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}
		objs := make([]*testObject, _10m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[0].Values[0].ids = append(s.individualItems[0].Values[0].ids, uint64(i))
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_10m)])
		}
	})
	b.Run("10,000,000 different individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m))
		objs := make([]*testObject, _10m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[uint64(i)] = &MultiValueItem[int]{Values: []*Value[int]{{val: i, ids: []uint64{uint64(i)}}}}
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_10m)])
		}
	})
	b.Run("10,000,000 shared items and 10,000,000 equal appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m))
		s.appendedItems = []*MultiValueItem[int]{{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}}
		objs := make([]*testObject, _10m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems[0].Values[0].ids = append(s.appendedItems[0].Values[0].ids, uint64(i))
			s.cachedLengths[uint64(i)] = _10m + 1
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_10m)])
		}
	})
	b.Run("10,000,000 shared items and 10,000,000 different appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m))
		s.appendedItems = []*MultiValueItem[int]{{}}
		objs := make([]*testObject, _10m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems[0].Values = append(s.appendedItems[0].Values, &Value[int]{val: i, ids: []uint64{uint64(i)}})
			s.cachedLengths[uint64(i)] = _10m + 1
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_10m)])
		}
	})
}

// compactValidator has the size and pointer-free layout of stateutil.CompactValidator.
type compactValidator [128]byte

func BenchmarkValueMainnet(b *testing.B) {
	b.Run("validators", func(b *testing.B) {
		benchmarkValueMainnet(b, func(i int) compactValidator { return compactValidator{byte(i), byte(i >> 8)} })
	})
	b.Run("balances", func(b *testing.B) {
		benchmarkValueMainnet(b, func(i int) uint64 { return uint64(i) })
	})
}

// benchmarkValueMainnet reads 2.34M items, the size of the mainnet validator registry.
// val must return a non-zero value for a positive input.
func benchmarkValueMainnet[V comparable](b *testing.B, val func(int) V) {
	const n = 2_340_000
	obj := &testObject{id: 1}
	b.Run("empty overlay", func(b *testing.B) {
		s := &Slice[V]{}
		s.Init(make([]V, n))
		b.ReportAllocs()
		for b.Loop() {
			s.Value(obj)
		}
	})
	b.Run("1000 individual and 10 appended items", func(b *testing.B) {
		s := &Slice[V]{}
		s.Init(make([]V, n))
		for i := range 1000 {
			require.NoError(b, s.UpdateAt(obj, uint64(i*1000), val(i+1)))
		}
		for i := range 10 {
			s.Append(obj, val(i+1))
		}
		b.ReportAllocs()
		for b.Loop() {
			s.Value(obj)
		}
	})
}
