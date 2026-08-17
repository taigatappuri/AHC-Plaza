// Package inputanalysis は入力形式の設定に従ってケース入力から数値を抽出します。
package inputanalysis

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
)

const maxInputLineBytes = 16 << 20

type Result struct {
	Values map[string]float64
	Errors map[string]string
}

// Extract は物理行と空白区切りの列を1始まりで数え、設定済みの数値を抽出します。
func Extract(reader io.Reader, variables []config.InputVariableConfig) (Result, error) {
	result := Result{
		Values: make(map[string]float64, len(variables)),
		Errors: make(map[string]string),
	}
	if len(variables) == 0 {
		return result, nil
	}

	variablesByLine := make(map[int][]config.InputVariableConfig)
	maxLine := 0
	for _, variable := range variables {
		variablesByLine[variable.Line] = append(variablesByLine[variable.Line], variable)
		if variable.Line > maxLine {
			maxLine = variable.Line
		}
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxInputLineBytes)
	lineNumber := 0
	for lineNumber < maxLine && scanner.Scan() {
		lineNumber++
		lineVariables := variablesByLine[lineNumber]
		if len(lineVariables) == 0 {
			continue
		}
		fields := strings.Fields(scanner.Text())
		for _, variable := range lineVariables {
			if variable.Column > len(fields) {
				result.Errors[variable.Name] = fmt.Sprintf("%d行目に%d列目がありません", variable.Line, variable.Column)
				continue
			}
			value, err := strconv.ParseFloat(fields[variable.Column-1], 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				result.Errors[variable.Name] = fmt.Sprintf("%d行%d列を数値として読めません", variable.Line, variable.Column)
				continue
			}
			result.Values[variable.Name] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("入力ファイルを読み取れません: %w", err)
	}
	for _, variable := range variables {
		if variable.Line > lineNumber {
			result.Errors[variable.Name] = fmt.Sprintf("%d行目がありません", variable.Line)
		}
	}
	return result, nil
}
