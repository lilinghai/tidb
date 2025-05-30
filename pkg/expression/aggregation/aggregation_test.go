// Copyright 2018 PingCAP, Inc.
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

package aggregation

import (
	"math"
	"testing"

	"github.com/pingcap/tidb/pkg/expression"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/mysql"
	"github.com/pingcap/tidb/pkg/planner/cascades/base"
	"github.com/pingcap/tidb/pkg/planner/util"
	"github.com/pingcap/tidb/pkg/sessionctx/vardef"
	"github.com/pingcap/tidb/pkg/sessionctx/variable"
	"github.com/pingcap/tidb/pkg/types"
	"github.com/pingcap/tidb/pkg/util/chunk"
	"github.com/pingcap/tidb/pkg/util/mock"
	"github.com/stretchr/testify/require"
)

type mockAggFuncSuite struct {
	ctx     *mock.Context
	rows    []chunk.Row
	nullRow chunk.Row
}

func createAggFuncSuite() (s *mockAggFuncSuite) {
	s = new(mockAggFuncSuite)
	s.ctx = mock.NewContext()
	s.ctx.GetSessionVars().GlobalVarsAccessor = variable.NewMockGlobalAccessor4Tests()
	s.ctx.GetSessionVars().DivPrecisionIncrement = vardef.DefDivPrecisionIncrement
	s.rows = make([]chunk.Row, 0, 5050)
	for i := 1; i <= 100; i++ {
		for range i {
			s.rows = append(s.rows, chunk.MutRowFromDatums(types.MakeDatums(i)).ToRow())
		}
	}
	s.nullRow = chunk.MutRowFromDatums([]types.Datum{{}}).ToRow()
	return
}

func TestAvg(t *testing.T) {
	s := createAggFuncSuite()
	col := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}
	ctx := mock.NewContext()
	desc, err := NewAggFuncDesc(s.ctx, ast.AggFuncAvg, []expression.Expression{col}, false)
	require.NoError(t, err)
	avgFunc := desc.GetAggFunc(ctx)
	evalCtx := avgFunc.CreateContext(s.ctx)

	result := avgFunc.GetResult(evalCtx)
	require.True(t, result.IsNull())

	for _, row := range s.rows {
		err := avgFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
		require.NoError(t, err)
	}
	result = avgFunc.GetResult(evalCtx)
	needed := types.NewDecFromStringForTest("67.000000000000000000000000000000")
	require.True(t, result.GetMysqlDecimal().Compare(needed) == 0)
	err = avgFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, s.nullRow)
	require.NoError(t, err)
	result = avgFunc.GetResult(evalCtx)
	require.True(t, result.GetMysqlDecimal().Compare(needed) == 0)

	desc, err = NewAggFuncDesc(s.ctx, ast.AggFuncAvg, []expression.Expression{col}, true)
	require.NoError(t, err)
	distinctAvgFunc := desc.GetAggFunc(ctx)
	evalCtx = distinctAvgFunc.CreateContext(s.ctx)
	for _, row := range s.rows {
		err := distinctAvgFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
		require.NoError(t, err)
	}
	result = distinctAvgFunc.GetResult(evalCtx)
	needed = types.NewDecFromStringForTest("50.500000000000000000000000000000")
	require.True(t, result.GetMysqlDecimal().Compare(needed) == 0)
	partialResult := distinctAvgFunc.GetPartialResult(evalCtx)
	require.Equal(t, int64(100), partialResult[0].GetInt64())
	needed = types.NewDecFromStringForTest("5050")
	require.Equalf(t, 0, partialResult[1].GetMysqlDecimal().Compare(needed), "%v, %v ", result.GetMysqlDecimal(), needed)
}

func TestAvgFinalMode(t *testing.T) {
	s := createAggFuncSuite()
	rows := make([][]types.Datum, 0, 100)
	for i := 1; i <= 100; i++ {
		rows = append(rows, types.MakeDatums(i, types.NewDecFromInt(int64(i*i))))
	}
	ctx := mock.NewContext()
	cntCol := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}
	sumCol := &expression.Column{
		Index:   1,
		RetType: types.NewFieldType(mysql.TypeNewDecimal),
	}
	aggFunc, err := NewAggFuncDesc(s.ctx, ast.AggFuncAvg, []expression.Expression{cntCol, sumCol}, false)
	require.NoError(t, err)
	aggFunc.Mode = FinalMode
	avgFunc := aggFunc.GetAggFunc(ctx)
	evalCtx := avgFunc.CreateContext(s.ctx)

	for _, row := range rows {
		err := avgFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, chunk.MutRowFromDatums(row).ToRow())
		require.NoError(t, err)
	}
	result := avgFunc.GetResult(evalCtx)
	needed := types.NewDecFromStringForTest("67.000000000000000000000000000000")
	require.True(t, result.GetMysqlDecimal().Compare(needed) == 0)
}

func TestSum(t *testing.T) {
	s := createAggFuncSuite()
	col := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}
	ctx := mock.NewContext()
	desc, err := NewAggFuncDesc(s.ctx, ast.AggFuncSum, []expression.Expression{col}, false)
	require.NoError(t, err)
	sumFunc := desc.GetAggFunc(ctx)
	evalCtx := sumFunc.CreateContext(s.ctx)

	result := sumFunc.GetResult(evalCtx)
	require.True(t, result.IsNull())

	for _, row := range s.rows {
		err := sumFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
		require.NoError(t, err)
	}
	result = sumFunc.GetResult(evalCtx)
	needed := types.NewDecFromStringForTest("338350")
	require.True(t, result.GetMysqlDecimal().Compare(needed) == 0)
	err = sumFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, s.nullRow)
	require.NoError(t, err)
	result = sumFunc.GetResult(evalCtx)
	require.True(t, result.GetMysqlDecimal().Compare(needed) == 0)
	partialResult := sumFunc.GetPartialResult(evalCtx)
	require.True(t, partialResult[0].GetMysqlDecimal().Compare(needed) == 0)

	desc, err = NewAggFuncDesc(s.ctx, ast.AggFuncSum, []expression.Expression{col}, true)
	require.NoError(t, err)
	distinctSumFunc := desc.GetAggFunc(ctx)
	evalCtx = distinctSumFunc.CreateContext(s.ctx)
	for _, row := range s.rows {
		err := distinctSumFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
		require.NoError(t, err)
	}
	result = distinctSumFunc.GetResult(evalCtx)
	needed = types.NewDecFromStringForTest("5050")
	require.True(t, result.GetMysqlDecimal().Compare(needed) == 0)
}

func TestBitAnd(t *testing.T) {
	s := createAggFuncSuite()
	col := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}
	ctx := mock.NewContext()
	desc, err := NewAggFuncDesc(s.ctx, ast.AggFuncBitAnd, []expression.Expression{col}, false)
	require.NoError(t, err)
	bitAndFunc := desc.GetAggFunc(ctx)
	evalCtx := bitAndFunc.CreateContext(s.ctx)

	result := bitAndFunc.GetResult(evalCtx)
	require.Equal(t, uint64(math.MaxUint64), result.GetUint64())

	row := chunk.MutRowFromDatums(types.MakeDatums(1)).ToRow()
	err = bitAndFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitAndFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	err = bitAndFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, s.nullRow)
	require.NoError(t, err)
	result = bitAndFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	row = chunk.MutRowFromDatums(types.MakeDatums(1)).ToRow()
	err = bitAndFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitAndFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	row = chunk.MutRowFromDatums(types.MakeDatums(3)).ToRow()
	err = bitAndFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitAndFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	row = chunk.MutRowFromDatums(types.MakeDatums(2)).ToRow()
	err = bitAndFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitAndFunc.GetResult(evalCtx)
	require.Equal(t, uint64(0), result.GetUint64())
	partialResult := bitAndFunc.GetPartialResult(evalCtx)
	require.Equal(t, uint64(0), partialResult[0].GetUint64())

	// test bit_and( decimal )
	col.RetType = types.NewFieldType(mysql.TypeNewDecimal)
	bitAndFunc.ResetContext(s.ctx, evalCtx)

	result = bitAndFunc.GetResult(evalCtx)
	require.Equal(t, uint64(math.MaxUint64), result.GetUint64())

	var dec types.MyDecimal
	err = dec.FromString([]byte("1.234"))
	require.NoError(t, err)
	row = chunk.MutRowFromDatums(types.MakeDatums(&dec)).ToRow()
	err = bitAndFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitAndFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	err = dec.FromString([]byte("3.012"))
	require.NoError(t, err)
	row = chunk.MutRowFromDatums(types.MakeDatums(&dec)).ToRow()
	err = bitAndFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitAndFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	err = dec.FromString([]byte("2.12345678"))
	require.NoError(t, err)
	row = chunk.MutRowFromDatums(types.MakeDatums(&dec)).ToRow()
	err = bitAndFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitAndFunc.GetResult(evalCtx)
	require.Equal(t, uint64(0), result.GetUint64())
}

func TestBitOr(t *testing.T) {
	s := createAggFuncSuite()
	col := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}
	ctx := mock.NewContext()
	desc, err := NewAggFuncDesc(s.ctx, ast.AggFuncBitOr, []expression.Expression{col}, false)
	require.NoError(t, err)
	bitOrFunc := desc.GetAggFunc(ctx)
	evalCtx := bitOrFunc.CreateContext(s.ctx)

	result := bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(0), result.GetUint64())

	row := chunk.MutRowFromDatums(types.MakeDatums(1)).ToRow()
	err = bitOrFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	err = bitOrFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, s.nullRow)
	require.NoError(t, err)
	result = bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	row = chunk.MutRowFromDatums(types.MakeDatums(1)).ToRow()
	err = bitOrFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	row = chunk.MutRowFromDatums(types.MakeDatums(3)).ToRow()
	err = bitOrFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(3), result.GetUint64())

	row = chunk.MutRowFromDatums(types.MakeDatums(2)).ToRow()
	err = bitOrFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(3), result.GetUint64())
	partialResult := bitOrFunc.GetPartialResult(evalCtx)
	require.Equal(t, uint64(3), partialResult[0].GetUint64())

	// test bit_or( decimal )
	col.RetType = types.NewFieldType(mysql.TypeNewDecimal)
	bitOrFunc.ResetContext(s.ctx, evalCtx)

	result = bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(0), result.GetUint64())

	var dec types.MyDecimal
	err = dec.FromString([]byte("12.234"))
	require.NoError(t, err)
	row = chunk.MutRowFromDatums(types.MakeDatums(&dec)).ToRow()
	err = bitOrFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(12), result.GetUint64())

	err = dec.FromString([]byte("1.012"))
	require.NoError(t, err)
	row = chunk.MutRowFromDatums(types.MakeDatums(&dec)).ToRow()
	err = bitOrFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(13), result.GetUint64())
	err = dec.FromString([]byte("15.12345678"))
	require.NoError(t, err)

	row = chunk.MutRowFromDatums(types.MakeDatums(&dec)).ToRow()
	err = bitOrFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(15), result.GetUint64())

	err = dec.FromString([]byte("16.00"))
	require.NoError(t, err)
	row = chunk.MutRowFromDatums(types.MakeDatums(&dec)).ToRow()
	err = bitOrFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitOrFunc.GetResult(evalCtx)
	require.Equal(t, uint64(31), result.GetUint64())
}

func TestBitXor(t *testing.T) {
	s := createAggFuncSuite()
	col := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}
	ctx := mock.NewContext()
	desc, err := NewAggFuncDesc(s.ctx, ast.AggFuncBitXor, []expression.Expression{col}, false)
	require.NoError(t, err)
	bitXorFunc := desc.GetAggFunc(ctx)
	evalCtx := bitXorFunc.CreateContext(s.ctx)

	result := bitXorFunc.GetResult(evalCtx)
	require.Equal(t, uint64(0), result.GetUint64())

	row := chunk.MutRowFromDatums(types.MakeDatums(1)).ToRow()
	err = bitXorFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitXorFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	err = bitXorFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, s.nullRow)
	require.NoError(t, err)
	result = bitXorFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	row = chunk.MutRowFromDatums(types.MakeDatums(1)).ToRow()
	err = bitXorFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitXorFunc.GetResult(evalCtx)
	require.Equal(t, uint64(0), result.GetUint64())

	row = chunk.MutRowFromDatums(types.MakeDatums(3)).ToRow()
	err = bitXorFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitXorFunc.GetResult(evalCtx)
	require.Equal(t, uint64(3), result.GetUint64())

	row = chunk.MutRowFromDatums(types.MakeDatums(2)).ToRow()
	err = bitXorFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitXorFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())
	partialResult := bitXorFunc.GetPartialResult(evalCtx)
	require.Equal(t, uint64(1), partialResult[0].GetUint64())

	// test bit_xor( decimal )
	col.RetType = types.NewFieldType(mysql.TypeNewDecimal)
	bitXorFunc.ResetContext(s.ctx, evalCtx)

	result = bitXorFunc.GetResult(evalCtx)
	require.Equal(t, uint64(0), result.GetUint64())

	var dec types.MyDecimal
	err = dec.FromString([]byte("1.234"))
	require.NoError(t, err)
	row = chunk.MutRowFromDatums(types.MakeDatums(&dec)).ToRow()
	err = bitXorFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitXorFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	err = dec.FromString([]byte("1.012"))
	require.NoError(t, err)
	row = chunk.MutRowFromDatums(types.MakeDatums(&dec)).ToRow()
	err = bitXorFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitXorFunc.GetResult(evalCtx)
	require.Equal(t, uint64(0), result.GetUint64())

	err = dec.FromString([]byte("2.12345678"))
	require.NoError(t, err)
	row = chunk.MutRowFromDatums(types.MakeDatums(&dec)).ToRow()
	err = bitXorFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = bitXorFunc.GetResult(evalCtx)
	require.Equal(t, uint64(2), result.GetUint64())
}

func TestCount(t *testing.T) {
	s := createAggFuncSuite()
	col := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}
	ctx := mock.NewContext()
	desc, err := NewAggFuncDesc(s.ctx, ast.AggFuncCount, []expression.Expression{col}, false)
	require.NoError(t, err)
	countFunc := desc.GetAggFunc(ctx)
	evalCtx := countFunc.CreateContext(s.ctx)

	result := countFunc.GetResult(evalCtx)
	require.Equal(t, int64(0), result.GetInt64())

	for _, row := range s.rows {
		err := countFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
		require.NoError(t, err)
	}
	result = countFunc.GetResult(evalCtx)
	require.Equal(t, int64(5050), result.GetInt64())
	err = countFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, s.nullRow)
	require.NoError(t, err)
	result = countFunc.GetResult(evalCtx)
	require.Equal(t, int64(5050), result.GetInt64())
	partialResult := countFunc.GetPartialResult(evalCtx)
	require.Equal(t, int64(5050), partialResult[0].GetInt64())

	desc, err = NewAggFuncDesc(s.ctx, ast.AggFuncCount, []expression.Expression{col}, true)
	require.NoError(t, err)
	distinctCountFunc := desc.GetAggFunc(ctx)
	evalCtx = distinctCountFunc.CreateContext(s.ctx)

	for _, row := range s.rows {
		err := distinctCountFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
		require.NoError(t, err)
	}
	result = distinctCountFunc.GetResult(evalCtx)
	require.Equal(t, int64(100), result.GetInt64())
}

func TestConcat(t *testing.T) {
	s := createAggFuncSuite()
	col := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}
	sep := &expression.Column{
		Index:   1,
		RetType: types.NewFieldType(mysql.TypeVarchar),
	}
	ctx := mock.NewContext()
	desc, err := NewAggFuncDesc(s.ctx, ast.AggFuncGroupConcat, []expression.Expression{col, sep}, false)
	require.NoError(t, err)
	concatFunc := desc.GetAggFunc(ctx)
	evalCtx := concatFunc.CreateContext(s.ctx)

	result := concatFunc.GetResult(evalCtx)
	require.True(t, result.IsNull())

	row := chunk.MutRowFromDatums(types.MakeDatums(1, "x"))
	err = concatFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = concatFunc.GetResult(evalCtx)
	require.Equal(t, "1", result.GetString())

	row.SetDatum(0, types.NewIntDatum(2))
	err = concatFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = concatFunc.GetResult(evalCtx)
	require.Equal(t, "1x2", result.GetString())

	row.SetDatum(0, types.NewDatum(nil))
	err = concatFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = concatFunc.GetResult(evalCtx)
	require.Equal(t, "1x2", result.GetString())
	partialResult := concatFunc.GetPartialResult(evalCtx)
	require.Equal(t, "1x2", partialResult[0].GetString())

	desc, err = NewAggFuncDesc(s.ctx, ast.AggFuncGroupConcat, []expression.Expression{col, sep}, true)
	require.NoError(t, err)
	distinctConcatFunc := desc.GetAggFunc(ctx)
	evalCtx = distinctConcatFunc.CreateContext(s.ctx)

	row.SetDatum(0, types.NewIntDatum(1))
	err = distinctConcatFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = distinctConcatFunc.GetResult(evalCtx)
	require.Equal(t, "1", result.GetString())

	row.SetDatum(0, types.NewIntDatum(1))
	err = distinctConcatFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = distinctConcatFunc.GetResult(evalCtx)
	require.Equal(t, "1", result.GetString())
}

func TestFirstRow(t *testing.T) {
	s := createAggFuncSuite()
	col := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}

	ctx := mock.NewContext()
	desc, err := NewAggFuncDesc(s.ctx, ast.AggFuncFirstRow, []expression.Expression{col}, false)
	require.NoError(t, err)
	firstRowFunc := desc.GetAggFunc(ctx)
	evalCtx := firstRowFunc.CreateContext(s.ctx)

	row := chunk.MutRowFromDatums(types.MakeDatums(1)).ToRow()
	err = firstRowFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result := firstRowFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())

	row = chunk.MutRowFromDatums(types.MakeDatums(2)).ToRow()
	err = firstRowFunc.Update(evalCtx, s.ctx.GetSessionVars().StmtCtx, row)
	require.NoError(t, err)
	result = firstRowFunc.GetResult(evalCtx)
	require.Equal(t, uint64(1), result.GetUint64())
	partialResult := firstRowFunc.GetPartialResult(evalCtx)
	require.Equal(t, uint64(1), partialResult[0].GetUint64())
}

func TestMaxMin(t *testing.T) {
	s := createAggFuncSuite()
	col := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}

	ctx := mock.NewContext()
	desc, err := NewAggFuncDesc(s.ctx, ast.AggFuncMax, []expression.Expression{col}, false)
	require.NoError(t, err)
	maxFunc := desc.GetAggFunc(ctx)
	desc, err = NewAggFuncDesc(s.ctx, ast.AggFuncMin, []expression.Expression{col}, false)
	require.NoError(t, err)
	minFunc := desc.GetAggFunc(ctx)
	maxEvalCtx := maxFunc.CreateContext(s.ctx)
	minEvalCtx := minFunc.CreateContext(s.ctx)

	result := maxFunc.GetResult(maxEvalCtx)
	require.True(t, result.IsNull())
	result = minFunc.GetResult(minEvalCtx)
	require.True(t, result.IsNull())

	row := chunk.MutRowFromDatums(types.MakeDatums(2))
	err = maxFunc.Update(maxEvalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = maxFunc.GetResult(maxEvalCtx)
	require.Equal(t, int64(2), result.GetInt64())
	err = minFunc.Update(minEvalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = minFunc.GetResult(minEvalCtx)
	require.Equal(t, int64(2), result.GetInt64())

	row.SetDatum(0, types.NewIntDatum(3))
	err = maxFunc.Update(maxEvalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = maxFunc.GetResult(maxEvalCtx)
	require.Equal(t, int64(3), result.GetInt64())
	err = minFunc.Update(minEvalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = minFunc.GetResult(minEvalCtx)
	require.Equal(t, int64(2), result.GetInt64())

	row.SetDatum(0, types.NewIntDatum(1))
	err = maxFunc.Update(maxEvalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = maxFunc.GetResult(maxEvalCtx)
	require.Equal(t, int64(3), result.GetInt64())
	err = minFunc.Update(minEvalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = minFunc.GetResult(minEvalCtx)
	require.Equal(t, int64(1), result.GetInt64())

	row.SetDatum(0, types.NewDatum(nil))
	err = maxFunc.Update(maxEvalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = maxFunc.GetResult(maxEvalCtx)
	require.Equal(t, int64(3), result.GetInt64())
	err = minFunc.Update(minEvalCtx, s.ctx.GetSessionVars().StmtCtx, row.ToRow())
	require.NoError(t, err)
	result = minFunc.GetResult(minEvalCtx)
	require.Equal(t, int64(1), result.GetInt64())
	partialResult := minFunc.GetPartialResult(minEvalCtx)
	require.Equal(t, int64(1), partialResult[0].GetInt64())
}

func TestAggFuncDesc(t *testing.T) {
	s := createAggFuncSuite()
	col := &expression.Column{
		Index:   0,
		RetType: types.NewFieldType(mysql.TypeLonglong),
	}
	desc1, err := NewAggFuncDesc(s.ctx, ast.AggFuncSum, []expression.Expression{col}, false)
	require.NoError(t, err)
	desc2, err := NewAggFuncDesc(s.ctx, ast.AggFuncSum, []expression.Expression{col}, false)
	require.NoError(t, err)
	hasher1 := base.NewHashEqualer()
	hasher2 := base.NewHashEqualer()
	desc1.Hash64(hasher1)
	desc2.Hash64(hasher2)
	require.Equal(t, hasher1.Sum64(), hasher2.Sum64())

	desc2.HasDistinct = true
	hasher2.Reset()
	desc2.Hash64(hasher2)
	require.NotEqual(t, hasher1.Sum64(), hasher2.Sum64())

	desc2.HasDistinct = false
	desc2.Mode = FinalMode
	hasher2.Reset()
	desc2.Hash64(hasher2)
	require.NotEqual(t, hasher1.Sum64(), hasher2.Sum64())

	desc2.Mode = CompleteMode
	desc2.Name = "whatever"
	hasher2.Reset()
	desc2.Hash64(hasher2)
	require.NotEqual(t, hasher1.Sum64(), hasher2.Sum64())

	desc2.Name = ast.AggFuncSum
	desc2.Args = []expression.Expression{}
	hasher2.Reset()
	desc2.Hash64(hasher2)
	require.NotEqual(t, hasher1.Sum64(), hasher2.Sum64())

	desc2.Args = []expression.Expression{col}
	desc2.RetTp = types.NewFieldType(mysql.TypeNewDecimal)
	hasher2.Reset()
	desc2.Hash64(hasher2)
	require.NotEqual(t, hasher1.Sum64(), hasher2.Sum64())

	desc2.RetTp = types.NewFieldType(mysql.TypeLonglong)
	desc2.OrderByItems = []*util.ByItems{{Expr: col, Desc: true}}
	hasher2.Reset()
	desc2.Hash64(hasher2)
	require.NotEqual(t, hasher1.Sum64(), hasher2.Sum64())
}
