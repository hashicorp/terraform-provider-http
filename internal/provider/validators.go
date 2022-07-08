package provider

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// urlWithSchemeAttributeValidator checks that a types.String attribute
// is indeed a URL and its scheme is one of the given `acceptableSchemes`.
//
// Instances should be created via UrlWithScheme function.
type urlWithSchemeAttributeValidator struct {
	acceptableSchemes []string
}

// UrlWithScheme is a helper to instantiate a urlWithSchemeAttributeValidator.
func UrlWithScheme(acceptableSchemes ...string) tfsdk.AttributeValidator {
	return &urlWithSchemeAttributeValidator{acceptableSchemes}
}

var _ tfsdk.AttributeValidator = (*urlWithSchemeAttributeValidator)(nil)

func (av *urlWithSchemeAttributeValidator) Description(ctx context.Context) string {
	return av.MarkdownDescription(ctx)
}

func (av *urlWithSchemeAttributeValidator) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("Ensures that the attribute is a URL and its scheme is one of: %q", av.acceptableSchemes)
}

func (av *urlWithSchemeAttributeValidator) Validate(ctx context.Context, req tfsdk.ValidateAttributeRequest, res *tfsdk.ValidateAttributeResponse) {
	if req.AttributeConfig.IsNull() || req.AttributeConfig.IsUnknown() {
		return
	}

	tflog.Debug(ctx, "Validating attribute value is a URL with acceptable scheme", map[string]interface{}{
		"attribute":         attrPathToString(req.AttributePath),
		"acceptableSchemes": strings.Join(av.acceptableSchemes, ","),
	})

	var v types.String
	diags := tfsdk.ValueAs(ctx, req.AttributeConfig, &v)
	if diags.HasError() {
		res.Diagnostics.Append(diags...)
		return
	}

	if v.IsNull() || v.IsUnknown() {
		return
	}

	u, err := url.Parse(v.Value)
	if err != nil {
		res.Diagnostics.AddAttributeError(
			req.AttributePath,
			"Invalid URL",
			fmt.Sprintf("Parsing URL %q failed: %v", v.Value, err),
		)
		return
	}

	if u.Host == "" {
		res.Diagnostics.AddAttributeError(
			req.AttributePath,
			"Invalid URL",
			fmt.Sprintf("URL %q contains no host", u.String()),
		)
		return
	}

	for _, s := range av.acceptableSchemes {
		if u.Scheme == s {
			return
		}
	}

	res.Diagnostics.AddAttributeError(
		req.AttributePath,
		"Invalid URL scheme",
		fmt.Sprintf("URL %q expected to use scheme from %q, got: %q", u.String(), av.acceptableSchemes, u.Scheme),
	)
}

// requiredWithAttributeValidator checks that a set of *tftypes.AttributePath,
// including the attribute it's applied to, are set simultaneously.
// This implements the validation logic declaratively within the tfsdk.Schema.
//
// The provided tftypes.AttributePath must be "absolute",
// and starting with top level attribute names.
type requiredWithAttributeValidator struct {
	attrPaths []*tftypes.AttributePath
}

// RequiredWith is a helper to instantiate requiredWithAttributeValidator.
func RequiredWith(attributePaths ...*tftypes.AttributePath) tfsdk.AttributeValidator {
	return &requiredWithAttributeValidator{attributePaths}
}

var _ tfsdk.AttributeValidator = (*requiredWithAttributeValidator)(nil)

func (av requiredWithAttributeValidator) Description(ctx context.Context) string {
	return av.MarkdownDescription(ctx)
}

func (av requiredWithAttributeValidator) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("Ensure that if an attribute is set, also these are set: %q", av.attrPaths)
}

func (av requiredWithAttributeValidator) Validate(ctx context.Context, req tfsdk.ValidateAttributeRequest, res *tfsdk.ValidateAttributeResponse) {
	tflog.Debug(ctx, "Validating attribute is set together with other required attributes", map[string]interface{}{
		"attribute":          attrPathToString(req.AttributePath),
		"requiredAttributes": av.attrPaths,
	})

	var v attr.Value
	res.Diagnostics.Append(tfsdk.ValueAs(ctx, req.AttributeConfig, &v)...)
	if res.Diagnostics.HasError() {
		return
	}

	for _, path := range av.attrPaths {
		var o attr.Value
		res.Diagnostics.Append(req.Config.GetAttribute(ctx, path, &o)...)
		if res.Diagnostics.HasError() {
			return
		}

		if !v.IsNull() && o.IsNull() {
			res.Diagnostics.AddAttributeError(
				req.AttributePath,
				fmt.Sprintf("Attribute %q missing", attrPathToString(path)),
				fmt.Sprintf("%q must be specified when %q is specified", attrPathToString(path), attrPathToString(req.AttributePath)),
			)
			return
		}
	}
}

// conflictsWithAttributeValidator checks that a set of *tftypes.AttributePath,
// including the attribute it's applied to, are not set simultaneously.
// This implements the validation logic declaratively within the tfsdk.Schema.
//
// The provided tftypes.AttributePath must be "absolute",
// and starting with top level attribute names.
type conflictsWithAttributeValidator struct {
	attrPaths []*tftypes.AttributePath
}

// ConflictsWith is a helper to instantiate conflictsWithAttributeValidator.
func ConflictsWith(attributePaths ...*tftypes.AttributePath) tfsdk.AttributeValidator {
	return &conflictsWithAttributeValidator{attributePaths}
}

var _ tfsdk.AttributeValidator = (*conflictsWithAttributeValidator)(nil)

func (av conflictsWithAttributeValidator) Description(ctx context.Context) string {
	return av.MarkdownDescription(ctx)
}

func (av conflictsWithAttributeValidator) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("Ensure that if an attribute is set, these are not set: %q", av.attrPaths)
}

func (av conflictsWithAttributeValidator) Validate(ctx context.Context, req tfsdk.ValidateAttributeRequest, res *tfsdk.ValidateAttributeResponse) {
	tflog.Debug(ctx, "Validating attribute is not set together with other conflicting attributes", map[string]interface{}{
		"attribute":             attrPathToString(req.AttributePath),
		"conflictingAttributes": av.attrPaths,
	})

	var v attr.Value
	res.Diagnostics.Append(tfsdk.ValueAs(ctx, req.AttributeConfig, &v)...)
	if res.Diagnostics.HasError() {
		return
	}

	for _, path := range av.attrPaths {
		var o attr.Value
		res.Diagnostics.Append(req.Config.GetAttribute(ctx, path, &o)...)
		if res.Diagnostics.HasError() {
			return
		}

		if !v.IsNull() && !o.IsNull() {
			res.Diagnostics.AddAttributeError(
				req.AttributePath,
				fmt.Sprintf("Attribute %q conflicting", attrPathToString(path)),
				fmt.Sprintf("%q cannot be specified when %q is specified", attrPathToString(path), attrPathToString(req.AttributePath)),
			)
			return
		}
	}
}

// attrPathToString takes all the tftypes.AttributePathStep in a tftypes.AttributePath and concatenates them,
// using `.` as separator.
//
// This should be used only when trying to "print out" a tftypes.AttributePath in a log or an error message.
func attrPathToString(path *tftypes.AttributePath) string {
	var res strings.Builder
	for pos, step := range path.Steps() {
		if pos != 0 {
			res.WriteString(".")
		}
		switch v := step.(type) {
		case tftypes.AttributeName:
			res.WriteString(string(v))
		case tftypes.ElementKeyString:
			res.WriteString(string(v))
		case tftypes.ElementKeyInt:
			res.WriteString(strconv.FormatInt(int64(v), 10))
		case tftypes.ElementKeyValue:
			res.WriteString(tftypes.Value(v).String())
		}
	}

	return res.String()
}
