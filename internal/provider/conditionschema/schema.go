// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package conditionschema

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

const operationDescription = "The comparison operator (for example equals, not_equals, present, blank). Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference)."

const valueDescription = "The value compared against. Omit for operators that take no value, such as present and blank."

// Attributes returns the all_of/any_of pair a condition is built from. Callers
// wrap it in a SingleNestedAttribute of their own, so each resource keeps
// control of whether its condition is required and how it documents it.
func Attributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"all_of": schema.ListNestedAttribute{
			Optional:    true,
			Description: "Comparisons that must all hold. Exactly one of all_of or any_of must be set.",
			Validators: []validator.List{
				listvalidator.SizeAtLeast(1),
				listvalidator.ExactlyOneOf(path.Expressions{path.MatchRelative().AtParent().AtName("any_of")}...),
			},
			NestedObject: schema.NestedAttributeObject{
				Attributes: comparisonAttributes(),
			},
		},
		"any_of": schema.ListNestedAttribute{
			Optional:    true,
			Description: "Comparisons or nested all_of groups where at least one must hold. Exactly one of all_of or any_of must be set. Takes at least 2 entries; a single entry is all_of.",
			Validators:  []validator.List{validators.ConditionAnyOfMinSize()},
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"field": schema.StringAttribute{
						Optional:    true,
						Description: "The internal_id of the field this entry compares, or a dotted path to a field reached through a connection. Omit when this entry is a nested all_of group instead.",
						Validators:  []validator.String{validators.NonBlank()},
					},
					"operation": schema.StringAttribute{
						Optional:    true,
						Description: operationDescription,
						Validators:  []validator.String{validators.NonBlank()},
					},
					"value": schema.StringAttribute{
						Optional:    true,
						Description: valueDescription,
						Validators:  []validator.String{validators.NonBlank()},
					},
					"all_of": schema.ListNestedAttribute{
						Optional:    true,
						Description: "A nested group of comparisons that must all hold, ORed against this entry's any_of siblings. Takes at least 2 comparisons; for a single one, set field, operation and value on the entry itself.",
						Validators:  []validator.List{listvalidator.SizeAtLeast(2)},
						NestedObject: schema.NestedAttributeObject{
							Attributes: comparisonAttributes(),
						},
					},
				},
				Validators: []validator.Object{validators.ConditionComparisonOrGroup()},
			},
		},
	}
}

func comparisonAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"field": schema.StringAttribute{
			Required:    true,
			Description: "The internal_id of the field this comparison evaluates. A dotted path addresses a field reached through a connection.",
			Validators:  []validator.String{validators.NonBlank()},
		},
		"operation": schema.StringAttribute{
			Required:    true,
			Description: operationDescription,
			Validators:  []validator.String{validators.NonBlank()},
		},
		"value": schema.StringAttribute{
			Optional:    true,
			Description: valueDescription,
			Validators:  []validator.String{validators.NonBlank()},
		},
	}
}
