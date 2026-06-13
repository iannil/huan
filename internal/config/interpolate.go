package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// envPattern matches ${VAR_NAME} or ${VAR_NAME:-default}.
//
// VAR_NAME starts with an uppercase letter or underscore, followed by
// uppercase letters/digits/underscores. The uppercase restriction is
// intentional: it prevents accidental matching of shell-style expressions
// in arbitrary content while still covering all conventional env var names
// (CLOUDFLARE_API_TOKEN, GH_SHA, etc.).
//
// The optional ":-default" suffix (POSIX shell syntax) provides a fallback
// value used when VAR_NAME is unset, instead of raising ErrEnvVarNotSet.
// The default may be any text except "}" (so ${VAR:-} is a legal empty
// default, but ${VAR:-${OTHER}} is not — nested interpolation is unsupported).
var envPattern = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)(?::-([^}]*))?\}`)

// ErrEnvVarNotSet is returned when a ${VAR} reference cannot be resolved
// because the env var is not set. Strict mode treats this as a fail-fast
// error to avoid silent empty-string substitution (a classic source of
// "apiToken: \"\" -> 401 -> debug for hours" bugs).
type ErrEnvVarNotSet struct {
	VarName string
}

func (e *ErrEnvVarNotSet) Error() string {
	return fmt.Sprintf("env var %q referenced by config but not set", e.VarName)
}

// Interpolate walks the parsed yaml tree (map[string]any / []any / string)
// and replaces ${VAR} occurrences in string leaves with env var values.
// Returns an error if any ${VAR} (without default) references an unset env
// var (strict mode).
//
// To make a field tolerate a missing env var, use ${VAR:-default}: when VAR
// is unset, the literal default (which may be empty) is substituted instead
// and no error is raised. This is the recommended way to mark optional
// plugin credentials — strict-by-default for the common case, opt-in lenient
// for fields whose consumers validate presence themselves.
//
// Type preservation: ${VAR} only makes sense in string fields. If used in
// int/bool fields, the resulting yaml unmarshal will fail with a type error.
// For numeric/bool config, use literal values directly.
//
// Env values are inserted as strings. yaml.Marshal will quote them if needed
// (e.g. when they contain ":" or "#"), so structural breakage is not possible
// even if an env value contains yaml-significant characters.
func Interpolate(raw map[string]any) (map[string]any, error) {
	out, err := walkValue(raw)
	if err != nil {
		return nil, err
	}
	m, ok := out.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("interpolate: expected map at root, got %T", out)
	}
	return m, nil
}

func walkValue(v any) (any, error) {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, sub := range val {
			interpolated, err := walkValue(sub)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", k, err)
			}
			out[k] = interpolated
		}
		return out, nil
	case []any:
		out := make([]any, len(val))
		for i, sub := range val {
			interpolated, err := walkValue(sub)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out[i] = interpolated
		}
		return out, nil
	case string:
		return interpolateString(val)
	case nil:
		return nil, nil
	default:
		return v, nil
	}
}

func interpolateString(s string) (string, error) {
	if !envPattern.MatchString(s) {
		return s, nil
	}
	var resolveErr error
	result := envPattern.ReplaceAllStringFunc(s, func(match string) string {
		if resolveErr != nil {
			return match
		}
		// match is "${...}"; strip "${" prefix and "}" suffix to get inner.
		inner := match[2 : len(match)-1]
		varName := inner
		defaultValue := ""
		hasDefault := false
		if idx := strings.Index(inner, ":-"); idx >= 0 {
			varName = inner[:idx]
			defaultValue = inner[idx+2:]
			hasDefault = true
		}
		value, ok := os.LookupEnv(varName)
		if !ok {
			if hasDefault {
				return defaultValue
			}
			resolveErr = &ErrEnvVarNotSet{VarName: varName}
			return match
		}
		return value
	})
	if resolveErr != nil {
		return "", resolveErr
	}
	return result, nil
}
