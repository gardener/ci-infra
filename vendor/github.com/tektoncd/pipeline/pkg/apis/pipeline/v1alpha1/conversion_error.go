/*
Copyright 2020 The Tekton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"knative.dev/pkg/apis"
)

const (
	// ConditionTypeConvertible is a Warning condition that is set on
	// resources when they cannot be converted to warn of a forthcoming
	// breakage.
	ConditionTypeConvertible apis.ConditionType = v1beta1.ConditionTypeConvertible
	// Conversion Error message for a field not available in v1alpha1
	ConversionErrorFieldNotAvailableMsg = "the specified field/section is not available in v1alpha1"
)

// CannotConvertError is returned when a field cannot be converted.
type CannotConvertError = v1beta1.CannotConvertError

var _ error = (*CannotConvertError)(nil)

// ConvertErrorf creates a CannotConvertError from the field name and format string.
var ConvertErrorf = v1beta1.ConvertErrorf
