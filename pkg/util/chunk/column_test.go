// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunk

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
	"unsafe"

	"github.com/pingcap/tidb/pkg/parser/mysql"
	"github.com/pingcap/tidb/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestColumnCopy(t *testing.T) {
	col := newFixedLenColumn(8, 10)
	for i := range 10 {
		col.AppendInt64(int64(i))
	}

	c1 := col.CopyConstruct(nil)
	require.Equal(t, col, c1)

	c2 := newFixedLenColumn(8, 10)
	c2 = col.CopyConstruct(c2)
	require.Equal(t, col, c2)
}

func TestColumnCopyReconstructFixedLen(t *testing.T) {
	col := NewColumn(types.NewFieldType(mysql.TypeLonglong), 1024)
	results := make([]int64, 0, 1024)
	nulls := make([]bool, 0, 1024)
	sel := make([]int, 0, 1024)
	for i := range 1024 {
		if rand.Intn(10) < 6 {
			sel = append(sel, i)
		}

		if rand.Intn(10) < 2 {
			col.AppendNull()
			nulls = append(nulls, true)
			results = append(results, 0)
			continue
		}

		v := rand.Int63()
		col.AppendInt64(v)
		results = append(results, v)
		nulls = append(nulls, false)
	}

	col = col.CopyReconstruct(sel, nil)
	nullCnt := 0
	for n, i := range sel {
		if nulls[i] {
			nullCnt++
			require.True(t, col.IsNull(n))
		} else {
			require.Equal(t, results[i], col.GetInt64(n))
		}
	}
	require.Equal(t, col.nullCount(), nullCnt)
	require.Len(t, sel, col.length)

	for i := range 128 {
		if i%2 == 0 {
			col.AppendNull()
		} else {
			col.AppendInt64(int64(i * i * i))
		}
	}

	require.Len(t, sel, col.length-128)
	require.Equal(t, nullCnt+128/2, col.nullCount())
	for i := range 128 {
		if i%2 == 0 {
			require.True(t, col.IsNull(len(sel)+i))
		} else {
			require.Equal(t, int64(i*i*i), col.GetInt64(len(sel)+i))
			require.False(t, col.IsNull(len(sel)+i))
		}
	}
}

func TestColumnCopyReconstructVarLen(t *testing.T) {
	col := NewColumn(types.NewFieldType(mysql.TypeVarString), 1024)
	results := make([]string, 0, 1024)
	nulls := make([]bool, 0, 1024)
	sel := make([]int, 0, 1024)
	for i := range 1024 {
		if rand.Intn(10) < 6 {
			sel = append(sel, i)
		}

		if rand.Intn(10) < 2 {
			col.AppendNull()
			nulls = append(nulls, true)
			results = append(results, "")
			continue
		}

		v := fmt.Sprintf("%v", rand.Int63())
		col.AppendString(v)
		results = append(results, v)
		nulls = append(nulls, false)
	}

	col = col.CopyReconstruct(sel, nil)
	nullCnt := 0
	for n, i := range sel {
		if nulls[i] {
			nullCnt++
			require.True(t, col.IsNull(n))
		} else {
			require.Equal(t, results[i], col.GetString(n))
		}
	}
	require.Equal(t, col.nullCount(), nullCnt)
	require.Len(t, sel, col.length)

	for i := range 128 {
		if i%2 == 0 {
			col.AppendNull()
		} else {
			col.AppendString(fmt.Sprintf("%v", i*i*i))
		}
	}

	require.Len(t, sel, col.length-128)
	require.Equal(t, nullCnt+128/2, col.nullCount())
	for i := range 128 {
		if i%2 == 0 {
			require.True(t, col.IsNull(len(sel)+i))
		} else {
			require.Equal(t, fmt.Sprintf("%v", i*i*i), col.GetString(len(sel)+i))
			require.False(t, col.IsNull(len(sel)+i))
		}
	}
}

func TestLargeStringColumnOffset(t *testing.T) {
	numRows := 1
	col := newVarLenColumn(numRows)
	// The max-length of a string field can be 6M, a typical batch size for Chunk is 1024, which is 1K.
	// That is to say, the memory offset of a string column can be 6GB, which exceeds int32
	col.offsets[0] = 6 << 30
	require.Equal(t, int64(6<<30), col.offsets[0]) // test no overflow.
}

func TestI64Column(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeLonglong)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		col.AppendInt64(int64(i))
	}

	i64s := col.Int64s()
	for i := range 1024 {
		require.Equal(t, int64(i), i64s[i])
		i64s[i]++
	}

	it := NewIterator4Chunk(chk)
	var i int
	for row := it.Begin(); row != it.End(); row = it.Next() {
		require.Equal(t, int64(i+1), row.GetInt64(0))
		require.Equal(t, int64(i+1), col.GetInt64(i))
		i++
	}
}

func TestF64Column(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeDouble)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		col.AppendFloat64(float64(i))
	}

	f64s := col.Float64s()
	for i := range 1024 {
		require.Equal(t, float64(i), f64s[i])
		f64s[i] /= 2
	}

	it := NewIterator4Chunk(chk)
	var i int64
	for row := it.Begin(); row != it.End(); row = it.Next() {
		require.Equal(t, float64(i)/2, row.GetFloat64(0))
		require.Equal(t, float64(i)/2, col.GetFloat64(int(i)))
		i++
	}
}

func TestF32Column(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeFloat)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		col.AppendFloat32(float32(i))
	}

	f32s := col.Float32s()
	for i := range 1024 {
		require.Equal(t, float32(i), f32s[i])
		f32s[i] /= 2
	}

	it := NewIterator4Chunk(chk)
	var i int64
	for row := it.Begin(); row != it.End(); row = it.Next() {
		require.Equal(t, float32(i)/2, row.GetFloat32(0))
		require.Equal(t, float32(i)/2, col.GetFloat32(int(i)))
		i++
	}
}

func TestDurationSliceColumn(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeDuration)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		col.AppendDuration(types.Duration{Duration: time.Duration(i)})
	}

	ds := col.GoDurations()
	for i := range 1024 {
		require.Equal(t, time.Duration(i), ds[i])
		d := types.Duration{Duration: ds[i]}
		d, _ = d.Add(d)
		ds[i] = d.Duration
	}

	it := NewIterator4Chunk(chk)
	var i int64
	for row := it.Begin(); row != it.End(); row = it.Next() {
		require.Equal(t, time.Duration(i)*2, row.GetDuration(0, 0).Duration)
		require.Equal(t, time.Duration(i)*2, col.GetDuration(int(i), 0).Duration)
		i++
	}
}

func TestMyDecimal(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeNewDecimal)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		d := new(types.MyDecimal)
		err := d.FromFloat64(float64(i) * 1.1)
		require.NoError(t, err)
		col.AppendMyDecimal(d)
	}

	ds := col.Decimals()
	for i := range 1024 {
		d := new(types.MyDecimal)
		err := d.FromFloat64(float64(i) * 1.1)
		require.NoError(t, err)
		require.Zero(t, d.Compare(&ds[i]))

		types.DecimalAdd(&ds[i], d, &ds[i])
		require.NoError(t, err)
	}

	it := NewIterator4Chunk(chk)
	var i int64
	for row := it.Begin(); row != it.End(); row = it.Next() {
		d := new(types.MyDecimal)
		err := d.FromFloat64(float64(i) * 1.1 * 2)
		require.NoError(t, err)

		delta := new(types.MyDecimal)
		err = types.DecimalSub(d, row.GetMyDecimal(0), delta)
		require.NoError(t, err)

		fDelta, err := delta.ToFloat64()
		require.NoError(t, err)
		require.InDelta(t, 0, fDelta, 0.0001)

		i++
	}
}

func TestStringColumn(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeVarString)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		col.AppendString(fmt.Sprintf("%v", i*i))
	}

	it := NewIterator4Chunk(chk)
	var i int
	for row := it.Begin(); row != it.End(); row = it.Next() {
		require.Equal(t, fmt.Sprintf("%v", i*i), row.GetString(0))
		require.Equal(t, fmt.Sprintf("%v", i*i), col.GetString(i))
		i++
	}
}

func TestSetColumn(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeSet)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		col.AppendSet(types.Set{Name: fmt.Sprintf("%v", i), Value: uint64(i)})
	}

	it := NewIterator4Chunk(chk)
	var i int
	for row := it.Begin(); row != it.End(); row = it.Next() {
		s1 := col.GetSet(i)
		s2 := row.GetSet(0)
		require.Equal(t, s2.Name, s1.Name)
		require.Equal(t, s2.Value, s1.Value)
		require.Equal(t, fmt.Sprintf("%v", i), s1.Name)
		require.Equal(t, uint64(i), s1.Value)
		i++
	}
}

func TestJSONColumn(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeJSON)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		j := new(types.BinaryJSON)
		err := j.UnmarshalJSON(fmt.Appendf(nil, `{"%v":%v}`, i, i))
		require.NoError(t, err)
		col.AppendJSON(*j)
	}

	it := NewIterator4Chunk(chk)
	var i int
	for row := it.Begin(); row != it.End(); row = it.Next() {
		j1 := col.GetJSON(i)
		j2 := row.GetJSON(0)
		require.Equal(t, j2.String(), j1.String())
		i++
	}
}

func TestTimeColumn(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeDatetime)}, 1024)
	col := chk.Column(0)
	for range 1024 {
		col.AppendTime(types.CurrentTime(mysql.TypeDatetime))
		time.Sleep(time.Millisecond / 10)
	}

	it := NewIterator4Chunk(chk)
	ts := col.Times()
	var i int
	for row := it.Begin(); row != it.End(); row = it.Next() {
		j1 := col.GetTime(i)
		j2 := row.GetTime(0)
		j3 := ts[i]
		require.Zero(t, j1.Compare(j2))
		require.Zero(t, j1.Compare(j3))
		i++
	}
}

func TestDurationColumn(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeDuration)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		col.AppendDuration(types.Duration{Duration: time.Second * time.Duration(i)})
	}

	it := NewIterator4Chunk(chk)
	var i int
	for row := it.Begin(); row != it.End(); row = it.Next() {
		j1 := col.GetDuration(i, 0)
		j2 := row.GetDuration(0, 0)
		require.Zero(t, j1.Compare(j2))
		i++
	}
}

func TestEnumColumn(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeEnum)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		col.AppendEnum(types.Enum{Name: fmt.Sprintf("%v", i), Value: uint64(i)})
	}

	it := NewIterator4Chunk(chk)
	var i int
	for row := it.Begin(); row != it.End(); row = it.Next() {
		s1 := col.GetEnum(i)
		s2 := row.GetEnum(0)
		require.Equal(t, s2.Name, s1.Name)
		require.Equal(t, s2.Value, s1.Value)
		require.Equal(t, fmt.Sprintf("%v", i), s1.Name)
		require.Equal(t, uint64(i), s1.Value)
		i++
	}
}

func TestNullsColumn(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeLonglong)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		if i%2 == 0 {
			col.AppendNull()
			continue
		}
		col.AppendInt64(int64(i))
	}

	it := NewIterator4Chunk(chk)
	var i int
	for row := it.Begin(); row != it.End(); row = it.Next() {
		if i%2 == 0 {
			require.True(t, row.IsNull(0))
			require.True(t, col.IsNull(i))
		} else {
			require.Equal(t, int64(i), row.GetInt64(0))
		}
		i++
	}
}

func TestReconstructFixedLen(t *testing.T) {
	col := NewColumn(types.NewFieldType(mysql.TypeLonglong), 1024)
	results := make([]int64, 0, 1024)
	nulls := make([]bool, 0, 1024)
	sel := make([]int, 0, 1024)
	for i := range 1024 {
		if rand.Intn(10) < 6 {
			sel = append(sel, i)
		}

		if rand.Intn(10) < 2 {
			col.AppendNull()
			nulls = append(nulls, true)
			results = append(results, 0)
			continue
		}

		v := rand.Int63()
		col.AppendInt64(v)
		results = append(results, v)
		nulls = append(nulls, false)
	}

	col.reconstruct(sel)
	nullCnt := 0
	for n, i := range sel {
		if nulls[i] {
			nullCnt++
			require.True(t, col.IsNull(n))
		} else {
			require.Equal(t, results[i], col.GetInt64(n))
		}
	}
	require.Equal(t, col.nullCount(), nullCnt)
	require.Len(t, sel, col.length)

	for i := range 128 {
		if i%2 == 0 {
			col.AppendNull()
		} else {
			col.AppendInt64(int64(i * i * i))
		}
	}

	require.Len(t, sel, col.length-128)
	require.Equal(t, nullCnt+128/2, col.nullCount())
	for i := range 128 {
		if i%2 == 0 {
			require.True(t, col.IsNull(len(sel)+i))
		} else {
			require.Equal(t, int64(i*i*i), col.GetInt64(len(sel)+i))
			require.False(t, col.IsNull(len(sel)+i))
		}
	}
}

func TestReconstructVarLen(t *testing.T) {
	col := NewColumn(types.NewFieldType(mysql.TypeVarString), 1024)
	results := make([]string, 0, 1024)
	nulls := make([]bool, 0, 1024)
	sel := make([]int, 0, 1024)
	for i := range 1024 {
		if rand.Intn(10) < 6 {
			sel = append(sel, i)
		}

		if rand.Intn(10) < 2 {
			col.AppendNull()
			nulls = append(nulls, true)
			results = append(results, "")
			continue
		}

		v := fmt.Sprintf("%v", rand.Int63())
		col.AppendString(v)
		results = append(results, v)
		nulls = append(nulls, false)
	}

	col.reconstruct(sel)
	nullCnt := 0
	for n, i := range sel {
		if nulls[i] {
			nullCnt++
			require.True(t, col.IsNull(n))
		} else {
			require.Equal(t, results[i], col.GetString(n))
		}
	}
	require.Equal(t, col.nullCount(), nullCnt)
	require.Len(t, sel, col.length)

	for i := range 128 {
		if i%2 == 0 {
			col.AppendNull()
		} else {
			col.AppendString(fmt.Sprintf("%v", i*i*i))
		}
	}

	require.Len(t, sel, col.length-128)
	require.Equal(t, nullCnt+128/2, col.nullCount())
	for i := range 128 {
		if i%2 == 0 {
			require.True(t, col.IsNull(len(sel)+i))
		} else {
			require.Equal(t, fmt.Sprintf("%v", i*i*i), col.GetString(len(sel)+i))
			require.False(t, col.IsNull(len(sel)+i))
		}
	}
}

func TestPreAllocInt64(t *testing.T) {
	col := NewColumn(types.NewFieldType(mysql.TypeLonglong), 128)
	col.ResizeInt64(256, true)
	i64s := col.Int64s()
	require.Equal(t, 256, len(i64s))
	for i := range 256 {
		require.True(t, col.IsNull(i))
	}
	col.AppendInt64(2333)
	require.False(t, col.IsNull(256))
	require.Equal(t, 257, len(col.Int64s()))
	require.Equal(t, int64(2333), col.Int64s()[256])
}

func TestPreAllocUint64(t *testing.T) {
	tll := types.NewFieldType(mysql.TypeLonglong)
	tll.AddFlag(mysql.UnsignedFlag)
	col := NewColumn(tll, 128)
	col.ResizeUint64(256, true)
	u64s := col.Uint64s()
	require.Equal(t, 256, len(u64s))
	for i := range 256 {
		require.True(t, col.IsNull(i))
	}
	col.AppendUint64(2333)
	require.False(t, col.IsNull(256))
	require.Equal(t, 257, len(col.Uint64s()))
	require.Equal(t, uint64(2333), col.Uint64s()[256])
}

func TestPreAllocFloat32(t *testing.T) {
	col := newFixedLenColumn(sizeFloat32, 128)
	col.ResizeFloat32(256, true)
	f32s := col.Float32s()
	require.Equal(t, 256, len(f32s))
	for i := range 256 {
		require.True(t, col.IsNull(i))
	}
	col.AppendFloat32(2333)
	require.False(t, col.IsNull(256))
	require.Equal(t, 257, len(col.Float32s()))
	require.Equal(t, float32(2333), col.Float32s()[256])
}

func TestPreAllocFloat64(t *testing.T) {
	col := newFixedLenColumn(sizeFloat64, 128)
	col.ResizeFloat64(256, true)
	f64s := col.Float64s()
	require.Equal(t, 256, len(f64s))
	for i := range 256 {
		require.True(t, col.IsNull(i))
	}
	col.AppendFloat64(2333)
	require.False(t, col.IsNull(256))
	require.Equal(t, 257, len(col.Float64s()))
	require.Equal(t, float64(2333), col.Float64s()[256])
}

func TestPreAllocDecimal(t *testing.T) {
	col := newFixedLenColumn(sizeMyDecimal, 128)
	col.ResizeDecimal(256, true)
	ds := col.Decimals()
	require.Equal(t, 256, len(ds))
	for i := range 256 {
		require.True(t, col.IsNull(i))
	}
	col.AppendMyDecimal(new(types.MyDecimal))
	require.False(t, col.IsNull(256))
	require.Equal(t, 257, len(col.Float64s()))
}

func TestPreAllocTime(t *testing.T) {
	col := newFixedLenColumn(sizeTime, 128)
	col.ResizeTime(256, true)
	ds := col.Times()
	require.Equal(t, 256, len(ds))
	for i := range 256 {
		require.True(t, col.IsNull(i))
	}
	col.AppendTime(types.ZeroDatetime)
	require.False(t, col.IsNull(256))
	require.Equal(t, 257, len(col.Times()))
}

func TestNull(t *testing.T) {
	col := newFixedLenColumn(sizeFloat64, 32)
	col.ResizeFloat64(1024, true)
	require.Equal(t, 1024, col.nullCount())

	notNulls := make(map[int]struct{})
	for range 512 {
		idx := rand.Intn(1024)
		notNulls[idx] = struct{}{}
		col.SetNull(idx, false)
	}

	require.Equal(t, 1024-len(notNulls), col.nullCount())
	for idx := range notNulls {
		require.False(t, col.IsNull(idx))
	}

	col.ResizeFloat64(8, true)
	col.SetNulls(0, 8, true)
	col.SetNull(7, false)
	require.Equal(t, 7, col.nullCount())

	col.ResizeFloat64(8, true)
	col.SetNulls(0, 8, true)
	require.Equal(t, 8, col.nullCount())

	col.ResizeFloat64(9, true)
	col.SetNulls(0, 9, true)
	col.SetNull(8, false)
	require.Equal(t, 8, col.nullCount())
}

func TestSetNulls(t *testing.T) {
	col := newFixedLenColumn(sizeFloat64, 32)
	col.ResizeFloat64(1024, true)
	require.Equal(t, 1024, col.nullCount())

	col.SetNulls(0, 1024, false)
	require.Zero(t, col.nullCount())

	nullMap := make(map[int]struct{})
	for range 100 {
		begin := rand.Intn(1024)
		l := rand.Intn(37)
		end := min(begin+l, 1024)
		for i := begin; i < end; i++ {
			nullMap[i] = struct{}{}
		}
		col.SetNulls(begin, end, true)

		require.Len(t, nullMap, col.nullCount())
		for k := range nullMap {
			require.True(t, col.IsNull(k))
		}
	}
}

func TestResizeReserve(t *testing.T) {
	cI64s := newFixedLenColumn(sizeInt64, 0)
	require.Zero(t, cI64s.length)
	for range 100 {
		n := rand.Intn(1024)
		cI64s.ResizeInt64(n, true)
		require.Equal(t, n, cI64s.length)
		require.Equal(t, n, len(cI64s.Int64s()))
	}
	cI64s.ResizeInt64(0, true)
	require.Zero(t, cI64s.length)
	require.Zero(t, len(cI64s.Int64s()))

	cStrs := newVarLenColumn(0)
	for range 100 {
		n := rand.Intn(1024)
		cStrs.ReserveString(n)
		require.Zero(t, cStrs.length)
	}
	cStrs.ReserveString(0)
	require.Zero(t, cStrs.length)
}

func TestGetRaw(t *testing.T) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeFloat)}, 1024)
	col := chk.Column(0)
	for i := range 1024 {
		col.AppendFloat32(float32(i))
	}
	it := NewIterator4Chunk(chk)
	var i int
	for row := it.Begin(); row != it.End(); row = it.Next() {
		f := float32(i)
		b := (*[unsafe.Sizeof(f)]byte)(unsafe.Pointer(&f))[:]
		require.Equal(t, b, row.GetRaw(0))
		require.Equal(t, b, col.GetRaw(i))
		i++
	}

	chk = NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeVarString)}, 1024)
	col = chk.Column(0)
	for i := range 1024 {
		col.AppendString(fmt.Sprint(i))
	}
	it = NewIterator4Chunk(chk)
	i = 0
	for row := it.Begin(); row != it.End(); row = it.Next() {
		require.Equal(t, fmt.Append(nil, i), row.GetRaw(0))
		require.Equal(t, fmt.Append(nil, i), col.GetRaw(i))
		i++
	}
}

func TestResize(t *testing.T) {
	col := NewColumn(types.NewFieldType(mysql.TypeLonglong), 1024)
	for i := range 1024 {
		col.AppendInt64(int64(i))
	}
	col.ResizeInt64(1024, false)
	for i := range 1024 {
		require.Equal(t, int64(0), col.Int64s()[i])
	}

	col = NewColumn(types.NewFieldType(mysql.TypeFloat), 1024)
	for i := range 1024 {
		col.AppendFloat32(float32(i))
	}
	col.ResizeFloat32(1024, false)
	for i := range 1024 {
		require.Equal(t, float32(0), col.Float32s()[i])
	}

	col = NewColumn(types.NewFieldType(mysql.TypeDouble), 1024)
	for i := range 1024 {
		col.AppendFloat64(float64(i))
	}
	col.ResizeFloat64(1024, false)
	for i := range 1024 {
		require.Equal(t, float64(0), col.Float64s()[i])
	}

	col = NewColumn(types.NewFieldType(mysql.TypeNewDecimal), 1024)
	for i := range 1024 {
		col.AppendMyDecimal(new(types.MyDecimal).FromInt(int64(i)))
	}
	col.ResizeDecimal(1024, false)
	for i := range 1024 {
		var d types.MyDecimal
		require.Equal(t, d, col.Decimals()[i])
	}

	col = NewColumn(types.NewFieldType(mysql.TypeDuration), 1024)
	for i := range 1024 {
		col.AppendDuration(types.Duration{Duration: time.Duration(i), Fsp: i})
	}
	col.ResizeGoDuration(1024, false)
	for i := range 1024 {
		require.Equal(t, time.Duration(0), col.GoDurations()[i])
	}

	col = NewColumn(types.NewFieldType(mysql.TypeDatetime), 1024)
	for range 1024 {
		gt := types.FromDate(rand.Intn(2200), rand.Intn(10)+1, rand.Intn(20)+1, rand.Intn(12), rand.Intn(60), rand.Intn(60), rand.Intn(1000000))
		t := types.NewTime(gt, 0, 0)
		col.AppendTime(t)
	}
	col.ResizeTime(1024, false)
	for i := range 1024 {
		var time types.Time
		require.Equal(t, time, col.Times()[i])
	}
}

func BenchmarkDurationRow(b *testing.B) {
	chk1 := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeDuration)}, 1024)
	col1 := chk1.Column(0)
	for i := range 1024 {
		col1.AppendDuration(types.Duration{Duration: time.Second * time.Duration(i)})
	}
	chk2 := chk1.CopyConstruct()
	result := chk1.CopyConstruct()

	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		result.Reset()
		it1 := NewIterator4Chunk(chk1)
		it2 := NewIterator4Chunk(chk2)
		for r1, r2 := it1.Begin(), it2.Begin(); r1 != it1.End() && r2 != it2.End(); r1, r2 = it1.Next(), it2.Next() {
			d1 := r1.GetDuration(0, 0)
			d2 := r2.GetDuration(0, 0)
			r, err := d1.Add(d2)
			if err != nil {
				b.Fatal(err)
			}
			result.AppendDuration(0, r)
		}
	}
}

func BenchmarkDurationVec(b *testing.B) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeDuration)}, 1024)
	col1 := chk.Column(0)
	for i := range 1024 {
		col1.AppendDuration(types.Duration{Duration: time.Second * time.Duration(i)})
	}
	col2 := col1.CopyConstruct(nil)
	result := col1.CopyConstruct(nil)

	ds1 := col1.GoDurations()
	ds2 := col2.GoDurations()
	rs := result.GoDurations()

	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		result.ResizeGoDuration(1024, true)
		for i := range 1024 {
			d1 := types.Duration{Duration: ds1[i]}
			d2 := types.Duration{Duration: ds2[i]}
			r, err := d1.Add(d2)
			if err != nil {
				b.Fatal(err)
			}
			rs[i] = r.Duration
		}
	}
}

func BenchmarkTimeRow(b *testing.B) {
	chk1 := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeDate)}, 1024)
	col1 := chk1.Column(0)
	for range 1024 {
		col1.AppendTime(types.ZeroDate)
	}
	chk2 := chk1.CopyConstruct()
	result := chk1.CopyConstruct()

	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		result.Reset()
		it1 := NewIterator4Chunk(chk1)
		it2 := NewIterator4Chunk(chk2)
		for r1, r2 := it1.Begin(), it2.Begin(); r1 != it1.End() && r2 != it2.End(); r1, r2 = it1.Next(), it2.Next() {
			d1 := r1.GetTime(0)
			d2 := r2.GetTime(0)
			if r := d1.Compare(d2); r > 0 {
				result.AppendTime(0, d1)
			} else {
				result.AppendTime(0, d2)
			}
		}
	}
}

func BenchmarkTimeVec(b *testing.B) {
	chk := NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeDate)}, 1024)
	col1 := chk.Column(0)
	for range 1024 {
		col1.AppendTime(types.ZeroDate)
	}
	col2 := col1.CopyConstruct(nil)
	result := col1.CopyConstruct(nil)

	ds1 := col1.Times()
	ds2 := col2.Times()
	rs := result.Times()

	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		result.ResizeTime(1024, true)
		for i := range 1024 {
			if r := ds1[i].Compare(ds2[i]); r > 0 {
				rs[i] = ds1[i]
			} else {
				rs[i] = ds2[i]
			}
		}
	}
}

func genNullCols(n int) []*Column {
	cols := make([]*Column, n)
	for i := range cols {
		cols[i] = NewColumn(types.NewFieldType(mysql.TypeLonglong), 1024)
		cols[i].ResizeInt64(1024, false)
		for j := range 1024 {
			if rand.Intn(10) < 5 {
				cols[i].SetNull(j, true)
			}
		}
	}
	return cols
}

func TestVectorizedNulls(t *testing.T) {
	for range 256 {
		cols := genNullCols(4)
		lCol, rCol := cols[0], cols[1]
		vecResult, rowResult := cols[2], cols[3]
		vecResult.SetNulls(0, 1024, false)
		rowResult.SetNulls(0, 1024, false)
		vecResult.MergeNulls(lCol, rCol)
		for i := range 1024 {
			rowResult.SetNull(i, lCol.IsNull(i) || rCol.IsNull(i))
		}

		for i := range 1024 {
			require.Equal(t, vecResult.IsNull(i), rowResult.IsNull(i))
		}
	}
}

func TestResetColumn(t *testing.T) {
	col0 := NewColumn(types.NewFieldType(mysql.TypeVarString), 0)
	col1 := NewColumn(types.NewFieldType(mysql.TypeLonglong), 0)

	// using col0.reset() here will cause panic since it doesn't reset the elemBuf field which
	// is used by MergeNulls.
	col0.Reset(types.ETInt)
	col0.MergeNulls(col1)

	col := NewColumn(types.NewFieldType(mysql.TypeDatetime), 0)
	col.Reset(types.ETDuration)
	col.AppendDuration(types.Duration{})
	// using col.reset() above will let this assertion fail since the length of initialized elemBuf
	// is sizeTime.
	require.Equal(t, sizeGoDuration, len(col.data))
}

func BenchmarkMergeNullsVectorized(b *testing.B) {
	cols := genNullCols(3)
	b.ResetTimer()
	for range b.N {
		cols[0].MergeNulls(cols[1:]...)
	}
}

func BenchmarkMergeNullsNonVectorized(b *testing.B) {
	cols := genNullCols(3)
	b.ResetTimer()
	for range b.N {
		for i := range 1024 {
			cols[0].SetNull(i, cols[1].IsNull(i) || cols[2].IsNull(i))
		}
	}
}

func TestColumnResizeInt64(t *testing.T) {
	var col = NewColumn(types.NewFieldType(mysql.TypeLonglong), 2)
	col.AppendUint64(11)
	col.AppendUint64(11)

	col.ResizeInt64(4, false)
	require.Equal(t, col.nullBitmap, []byte{0b1111})
	col.AppendUint64(11)
	require.Equal(t, col.nullBitmap, []byte{0b11111})
	col.AppendNull()
	require.Equal(t, col.nullBitmap, []byte{0b011111})

	col.ResizeUint64(11, false)
	require.Equal(t, col.nullBitmap, []byte{0b11111111, 0b111})

	col.ResizeUint64(7, true)
	require.Equal(t, col.nullBitmap, []byte{0})

	col.AppendUint64(32)
	col.AppendUint64(32)
	require.Equal(t, col.nullBitmap, []byte{0b10000000, 0b1})
}
