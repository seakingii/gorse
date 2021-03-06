package core

import (
	"github.com/zhenghaoz/gorse/base"
	"gonum.org/v1/gonum/stat"
	"gopkg.in/cheggaaa/pb.v1"
	"math"
	"reflect"
)

// ParameterGrid contains candidate for grid search.
type ParameterGrid map[base.ParamName][]interface{}

/* Cross Validation */

// CrossValidateResult contains the result of cross validate
type CrossValidateResult struct {
	TestScore []float64
	TestTime  []float64
	FitTime   []float64
}

// MeanAndMargin returns the mean and the margin of cross validation scores.
func (sv CrossValidateResult) MeanAndMargin() (float64, float64) {
	mean := stat.Mean(sv.TestScore, nil)
	margin := 0.0
	for _, score := range sv.TestScore {
		temp := math.Abs(score - mean)
		if temp > margin {
			margin = temp
		}
	}
	return mean, margin
}

// CrossValidate evaluates a model by k-fold cross validation.
func CrossValidate(estimator Model, dataSet Table, metrics []Evaluator,
	splitter Splitter, seed int64, options ...RuntimeOption) []CrossValidateResult {
	cvOptions := NewRuntimeOptions(options)
	// Split data set
	trainFolds, testFolds := splitter(dataSet, seed)
	length := len(trainFolds)
	// Create return structures
	ret := make([]CrossValidateResult, len(metrics))
	for i := 0; i < len(ret); i++ {
		ret[i].TestScore = make([]float64, length)
	}
	// Cross validation
	params := estimator.GetParams()
	base.Parallel(length, cvOptions.NJobs, func(begin, end int) {
		cp := reflect.New(reflect.TypeOf(estimator).Elem()).Interface().(Model)
		Copy(cp, estimator)
		for i := begin; i < end; i++ {
			trainFold := trainFolds[i]
			testFold := testFolds[i]
			cp.SetParams(params)
			cp.Fit(trainFold)
			// Evaluate on test set
			for j := 0; j < len(ret); j++ {
				ret[j].TestScore[i] = metrics[j](cp, testFold, trainFold)
			}
		}
	})
	return ret
}

/* Model Selection */

// ModelSelectionResult contains the return of grid search.
type ModelSelectionResult struct {
	BestScore  float64
	BestParams base.Params
	BestIndex  int
	CVResults  []CrossValidateResult
	AllParams  []base.Params
}

// GridSearchCV finds the best parameters for a model.
func GridSearchCV(estimator Model, dataSet Table, paramGrid ParameterGrid,
	evaluators []Evaluator, splitter Splitter, seed int64, options ...RuntimeOption) []ModelSelectionResult {
	// Retrieve parameter names and length
	paramNames := make([]base.ParamName, 0, len(paramGrid))
	count := 1
	for paramName, values := range paramGrid {
		paramNames = append(paramNames, paramName)
		count *= len(values)
	}
	// Create GridSearch result
	results := make([]ModelSelectionResult, len(evaluators))
	for i := range results {
		results[i] = ModelSelectionResult{}
		results[i].BestScore = math.Inf(1)
		results[i].CVResults = make([]CrossValidateResult, 0, count)
		results[i].AllParams = make([]base.Params, 0, count)
	}
	// Progress bar
	bar := pb.StartNew(count)
	// Construct DFS procedure
	var dfs func(deep int, params base.Params)
	dfs = func(deep int, params base.Params) {
		if deep == len(paramNames) {
			// Cross validate
			estimator.GetParams().Merge(params)
			estimator.SetParams(estimator.GetParams())
			cvResults := CrossValidate(estimator, dataSet, evaluators, splitter, seed, options...)
			for i := range cvResults {
				results[i].CVResults = append(results[i].CVResults, cvResults[i])
				results[i].AllParams = append(results[i].AllParams, params.Copy())
				score := stat.Mean(cvResults[i].TestScore, nil)
				if score < results[i].BestScore {
					results[i].BestScore = score
					results[i].BestParams = params.Copy()
					results[i].BestIndex = len(results[i].AllParams) - 1
				}
				// Progress bar
				bar.Increment()
			}
		} else {
			paramName := paramNames[deep]
			values := paramGrid[paramName]
			for _, val := range values {
				params[paramName] = val
				dfs(deep+1, params)
			}
		}
	}
	params := make(map[base.ParamName]interface{})
	dfs(0, params)
	bar.FinishPrint("Completed!")
	return results
}

// RandomSearchCV searches hyper-parameters by random.
func RandomSearchCV(estimator Model, dataSet Table, paramGrid ParameterGrid, evaluators []Evaluator,
	splitter Splitter, trial int, seed int64, options ...RuntimeOption) []ModelSelectionResult {
	rng := base.NewRandomGenerator(seed)
	// Create results
	results := make([]ModelSelectionResult, len(evaluators))
	for i := range results {
		results[i] = ModelSelectionResult{}
		results[i].BestScore = math.Inf(1)
		results[i].CVResults = make([]CrossValidateResult, trial)
		results[i].AllParams = make([]base.Params, trial)
	}
	for i := 0; i < trial; i++ {
		// Make parameters
		params := base.Params{}
		for paramName, values := range paramGrid {
			value := values[rng.Intn(len(values))]
			params[paramName] = value
		}
		// Cross validate
		estimator.GetParams().Merge(params)
		estimator.SetParams(estimator.GetParams())
		cvResults := CrossValidate(estimator, dataSet, evaluators, splitter, seed, options...)
		for j := range cvResults {
			results[j].CVResults[i] = cvResults[j]
			results[j].AllParams[i] = params.Copy()
			score := stat.Mean(cvResults[j].TestScore, nil)
			if score < results[j].BestScore {
				results[j].BestScore = score
				results[j].BestParams = params.Copy()
				results[j].BestIndex = len(results[j].AllParams) - 1
			}
		}
	}
	return results
}
