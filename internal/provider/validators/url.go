// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// URL returns a validator.String that ensures the value is a well-formed
// absolute URL with an "http" or "https" scheme and a host (e.g.
// "https://example.com/hook" or "http://localhost:3000/hook"). Plain HTTP is
// accepted so local endpoints can be used; backends that require HTTPS reject
// non-HTTPS URLs at apply time. Empty and unknown values are allowed through so
// optional / computed attributes are not rejected; required-attribute checks
// are handled by the schema itself.
func URL() validator.String {
	return urlValidator{}
}

type urlValidator struct{}

func (v urlValidator) Description(_ context.Context) string {
	return "value must be an http or https URL"
}

func (v urlValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v urlValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	value := req.ConfigValue.ValueString()
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid URL",
			"expected an http or https URL like https://example.com/hook, got: "+value,
		)
	}
}
