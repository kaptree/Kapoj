package problem

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	maxBatchTestcases  = 100
	maxBatchTotalBytes = 200 << 20
)

// BatchTestcaseFile is one uploaded file in a batch testcase request. The
// handler owns and closes Reader.
type BatchTestcaseFile struct {
	Name   string
	Size   int64
	Reader io.Reader
}

type batchTestcasePair struct {
	inputName, outputName string
	input, output         *BatchTestcaseFile
}

// AddTestcasesBatch pairs .in with .out/.ans, writes all files first, then
// inserts every testcase in one transaction. A failure leaves no partial data.
func (s *Service) AddTestcasesBatch(ctx context.Context, problemID int64, files []BatchTestcaseFile, score int) ([]*Testcase, error) {
	if score < 0 || score > maxScore {
		return nil, httpx.ErrValidation(fmt.Sprintf("分数需为 0–%d", maxScore))
	}
	if _, err := s.repo.FindByID(ctx, problemID); err != nil {
		return nil, mapRepoErr(err)
	}
	limits := s.limits(ctx)
	pairs, err := pairBatchTestcases(files, limits)
	if err != nil {
		return nil, err
	}

	cases := make([]*Testcase, 0, len(pairs))
	var keys []string
	var total int64
	cleanup := func() { s.deleteFiles(ctx, keys) }

	for _, pair := range pairs {
		base := fmt.Sprintf("problems/%d/testcases/%s", problemID, randomHex())
		tc := &Testcase{
			ProblemID: problemID, InputFile: base + ".in", OutputFile: base + ".out",
			Score: score,
		}
		tc.InputSize, err = s.put(ctx, tc.InputFile, pair.input.Reader, limits.MaxInputBytes, "输入文件")
		if err != nil {
			cleanup()
			return nil, err
		}
		keys = append(keys, tc.InputFile)
		tc.OutputSize, err = s.put(ctx, tc.OutputFile, pair.output.Reader, limits.MaxOutputBytes, "输出文件")
		if err != nil {
			cleanup()
			return nil, err
		}
		keys = append(keys, tc.OutputFile)
		total += tc.InputSize + tc.OutputSize
		if total > limits.MaxBatchTotalBytes {
			cleanup()
			return nil, httpx.ErrValidation(fmt.Sprintf("批量测试数据不能超过 %s", formatLimitBytes(limits.MaxBatchTotalBytes)))
		}
		cases = append(cases, tc)
	}
	if err := s.repo.AddTestcases(ctx, problemID, cases); err != nil {
		cleanup()
		return nil, mapRepoErr(err)
	}
	return cases, nil
}

func pairBatchTestcases(files []BatchTestcaseFile, limits TestcaseLimits) ([]batchTestcasePair, error) {
	type entry struct{ input, out, ans *BatchTestcaseFile }
	byBase := map[string]*entry{}
	var total int64
	for i := range files {
		file := &files[i]
		name := path.Base(strings.ReplaceAll(file.Name, "\\", "/"))
		lower := strings.ToLower(name)
		var base string
		var target **BatchTestcaseFile
		switch {
		case strings.HasSuffix(lower, ".in"):
			base, target = name[:len(name)-3], nil
			if _, ok := byBase[base]; !ok {
				byBase[base] = &entry{}
			}
			if byBase[base].input != nil {
				return nil, httpx.ErrValidation("存在重复的输入文件：" + name)
			}
			byBase[base].input = file
			continue
		case strings.HasSuffix(lower, ".out"):
			base = name[:len(name)-4]
			if _, ok := byBase[base]; !ok {
				byBase[base] = &entry{}
			}
			target = &byBase[base].out
		case strings.HasSuffix(lower, ".ans"):
			base = name[:len(name)-4]
			if _, ok := byBase[base]; !ok {
				byBase[base] = &entry{}
			}
			target = &byBase[base].ans
		default:
			continue
		}
		if base == "" {
			continue
		}
		if *target != nil {
			return nil, httpx.ErrValidation("存在重复的输出文件：" + name)
		}
		*target = file
	}
	if len(byBase) == 0 {
		return nil, httpx.ErrValidation("没有找到 .in 测试数据")
	}
	if len(byBase) > maxBatchTestcases {
		return nil, httpx.ErrValidation(fmt.Sprintf("一次最多上传 %d 对测试数据", maxBatchTestcases))
	}

	names := make([]string, 0, len(byBase))
	for name, e := range byBase {
		if e.input == nil || (e.out == nil && e.ans == nil) {
			return nil, httpx.ErrValidation("每个 .in 必须对应一个 .out 或 .ans：" + name)
		}
		if e.input.Size > limits.MaxInputBytes {
			return nil, httpx.ErrValidation(fmt.Sprintf("输入文件不能超过 %s：%s", formatLimitBytes(limits.MaxInputBytes), e.input.Name))
		}
		output := e.out
		if output == nil {
			output = e.ans
		}
		if output.Size > limits.MaxOutputBytes {
			return nil, httpx.ErrValidation(fmt.Sprintf("输出文件不能超过 %s：%s", formatLimitBytes(limits.MaxOutputBytes), output.Name))
		}
		total += e.input.Size + output.Size
		if total > limits.MaxBatchTotalBytes {
			return nil, httpx.ErrValidation(fmt.Sprintf("批量测试数据不能超过 %s", formatLimitBytes(limits.MaxBatchTotalBytes)))
		}
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return lessHydroTestbase(names[i], names[j]) })

	pairs := make([]batchTestcasePair, 0, len(names))
	for _, name := range names {
		e := byBase[name]
		output := e.out
		if output == nil {
			output = e.ans
		}
		pairs = append(pairs, batchTestcasePair{
			inputName: e.input.Name, outputName: output.Name,
			input: e.input, output: output,
		})
	}
	return pairs, nil
}
