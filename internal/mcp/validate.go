package mcp

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var (
	validIDRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
	validNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._/:-]{0,63}$`)
	envNameRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	// RFC 9110 field-name / RFC 5234 token: tchar = "!" / "#" / "$" / "%" / "&" /
	// "'" / "*" / "+" / "-" / "." / "^" / "_" / "`" / "|" / "~" / DIGIT / ALPHA
	httpHeaderNameRe = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]+$`)
)

// ValidateDefinition checks a normalized MCP definition.
func ValidateDefinition(def Definition) error {
	var errs []string
	id := strings.TrimSpace(def.ID)
	if id == "" {
		errs = append(errs, "id is required")
	} else if !validIDRe.MatchString(id) && def.Source != SourceCustom {
		// custom IDs are generated (custom-N); still require non-empty sanitized form
		errs = append(errs, fmt.Sprintf("id %q is invalid", id))
	} else if def.Source == SourceCustom && SanitizeNativeKey(id) == "" {
		errs = append(errs, "id is invalid")
	}
	name := strings.TrimSpace(def.DisplayName)
	if name == "" {
		errs = append(errs, "display name is required")
	} else if !validNameRe.MatchString(name) {
		errs = append(errs, fmt.Sprintf("display name %q is invalid", name))
	}

	transport := NormalizeTransport(string(def.Transport))
	if transport == TransportInvalid {
		errs = append(errs, fmt.Sprintf("unknown transport %q", def.Transport))
	}
	if def.Materializable {
		switch transport {
		case TransportStdio:
			if strings.TrimSpace(def.Command) == "" {
				errs = append(errs, "stdio transport requires command")
			}
			if looksLikeShellConcat(def.Command) {
				errs = append(errs, "command must not be a shell concatenation string")
			}
			for _, arg := range def.Args {
				if looksLikeShellConcat(arg) {
					errs = append(errs, "args must not contain shell concatenation")
					break
				}
			}
			if strings.TrimSpace(def.Endpoint) != "" {
				errs = append(errs, "stdio transport must not set endpoint")
			}
		case TransportStreamableHTTP:
			if err := validateHTTPURL(def.Endpoint); err != nil {
				errs = append(errs, err.Error())
			}
			if strings.TrimSpace(def.Command) != "" {
				errs = append(errs, "streamable_http transport must not set command")
			}
		case TransportSSE:
			// Legacy: allow load of old configs that still carry sse + URL.
			if strings.TrimSpace(def.Endpoint) == "" && strings.TrimSpace(def.Command) == "" {
				errs = append(errs, "legacy sse transport requires endpoint")
			}
			if ep := strings.TrimSpace(def.Endpoint); ep != "" {
				if err := validateHTTPURL(ep); err != nil {
					errs = append(errs, err.Error())
				}
			}
		case TransportInvalid:
			// already recorded above
		default:
			errs = append(errs, fmt.Sprintf("unknown transport %q", def.Transport))
		}
	}

	for _, ref := range def.EnvRefs {
		if err := validateEnvRef(ref); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for header, ref := range def.HeaderRefs {
		if err := validateHTTPHeaderName(header); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := validateHeaderValueRef(ref); err != nil {
			errs = append(errs, fmt.Sprintf("header %s: %v", header, err))
		}
	}

	switch def.AuthRequirement {
	case AuthNone, AuthEnvironmentReference, AuthOAuthExternal, AuthProviderManaged, "":
	default:
		errs = append(errs, fmt.Sprintf("unknown auth requirement %q", def.AuthRequirement))
	}

	if ContainsSecretMaterial(def.Command) || ContainsSecretMaterial(def.Endpoint) {
		errs = append(errs, "secret material must not appear in command/endpoint")
	}
	for _, arg := range def.Args {
		if ContainsSecretMaterial(arg) {
			errs = append(errs, "secret material must not appear in args")
			break
		}
	}
	for _, ref := range def.EnvRefs {
		if ContainsSecretMaterial(ref) {
			errs = append(errs, "env references must be names, not secret values")
			break
		}
	}

	if !def.Materializable && def.Source == SourceCustom {
		errs = append(errs, "custom MCP definitions must be materializable")
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid MCP definition %q: %s", id, strings.Join(errs, "; "))
}

// ValidateDesiredState validates all definitions in a desired state.
func ValidateDesiredState(state DesiredState) error {
	seen := map[string]struct{}{}
	var errs []string
	for _, def := range state.Definitions {
		if err := ValidateDefinition(def); err != nil {
			errs = append(errs, err.Error())
		}
		key := strings.ToLower(strings.TrimSpace(def.ID))
		if _, dup := seen[key]; dup {
			errs = append(errs, fmt.Sprintf("duplicate definition id %q", def.ID))
		}
		seen[key] = struct{}{}
	}
	if err := ValidateNativeKeyUniqueness(state.Definitions); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}

// ValidateNativeKeyUniqueness blocks materializable definitions that share a native key.
// Prevents silent overwrites in internal maps (e.g. builtin github + custom "GitHub").
func ValidateNativeKeyUniqueness(defs []Definition) error {
	byKey := map[string]string{} // nativeKey -> definition ID
	var errs []string
	for _, def := range defs {
		if !def.Materializable {
			continue
		}
		// Collision matters for any materializable definition in the desired set,
		// including disabled ones that could be enabled later in the same draft.
		key := NativeKeyFor(def)
		if key == "" {
			errs = append(errs, fmt.Sprintf("definition %q has empty native key", def.ID))
			continue
		}
		if prev, ok := byKey[key]; ok && prev != def.ID {
			errs = append(errs, fmt.Sprintf("native key collision %q between %q and %q", key, prev, def.ID))
			continue
		}
		byKey[key] = def.ID
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}

func validateHTTPURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("http transport requires URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("URL is invalid: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("URL host is required")
	}
	if strings.Contains(u.Path, "..") {
		return fmt.Errorf("URL path traversal is prohibited")
	}
	return nil
}

func validateEnvRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("environment reference is empty")
	}
	// Allow bare NAME or ${env:NAME} / {env:NAME} forms as references (not values).
	name := ref
	switch {
	case strings.HasPrefix(ref, "${env:") && strings.HasSuffix(ref, "}"):
		name = strings.TrimSuffix(strings.TrimPrefix(ref, "${env:"), "}")
	case strings.HasPrefix(ref, "{env:") && strings.HasSuffix(ref, "}"):
		name = strings.TrimSuffix(strings.TrimPrefix(ref, "{env:"), "}")
	}
	if !envNameRe.MatchString(name) {
		return fmt.Errorf("environment reference %q is invalid", ref)
	}
	if ContainsSecretMaterial(ref) {
		return fmt.Errorf("environment reference looks like a secret value")
	}
	return nil
}

// validateHTTPHeaderName enforces RFC 9110 field-name token syntax.
func validateHTTPHeaderName(name string) error {
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("header name %q is invalid", name)
	}
	if name == "" {
		return fmt.Errorf("header name is required")
	}
	if !httpHeaderNameRe.MatchString(name) {
		return fmt.Errorf("header name %q is invalid", name)
	}
	return nil
}

func validateHeaderValueRef(ref HeaderValueRef) error {
	if err := validateEnvRef(ref.Env); err != nil {
		return err
	}
	prefix := ref.Prefix
	if prefix == "" {
		return nil
	}
	if ContainsSecretMaterial(prefix) {
		return fmt.Errorf("header prefix must not contain secret material")
	}
	if looksLikeShellConcat(prefix) {
		return fmt.Errorf("header prefix is invalid")
	}
	// Prefix is a short literal (e.g. "Bearer "); reject long opaque blobs.
	if len(prefix) > 64 {
		return fmt.Errorf("header prefix is too long")
	}
	for _, r := range prefix {
		if r == '\n' || r == '\r' || r == '\x00' {
			return fmt.Errorf("header prefix contains invalid characters")
		}
	}
	return nil
}

func looksLikeShellConcat(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	// Allow known interpolation tokens used by agent MCP configs.
	stripped := stripInterpolationTokens(value)
	if strings.ContainsAny(stripped, "|&;<>`") {
		return true
	}
	if strings.Contains(stripped, "$(") || strings.Contains(stripped, "${") {
		return true
	}
	if strings.Contains(value, "&&") || strings.Contains(value, "||") {
		return true
	}
	// Disallow embedded newlines (command injection surface).
	for _, r := range value {
		if r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}

func stripInterpolationTokens(value string) string {
	// Cursor: ${workspaceFolder}, ${env:NAME}, ${userHome}, ...
	// OpenCode: {env:NAME}
	out := value
	for {
		start := strings.Index(out, "${")
		if start < 0 {
			break
		}
		end := strings.Index(out[start:], "}")
		if end < 0 {
			break
		}
		out = out[:start] + out[start+end+1:]
	}
	for {
		start := strings.Index(out, "{env:")
		if start < 0 {
			break
		}
		end := strings.Index(out[start:], "}")
		if end < 0 {
			break
		}
		out = out[:start] + out[start+end+1:]
	}
	return out
}

// EnvRefName extracts the bare environment variable name from a reference.
func EnvRefName(ref string) string {
	ref = strings.TrimSpace(ref)
	switch {
	case strings.HasPrefix(ref, "${env:") && strings.HasSuffix(ref, "}"):
		return strings.TrimSuffix(strings.TrimPrefix(ref, "${env:"), "}")
	case strings.HasPrefix(ref, "{env:") && strings.HasSuffix(ref, "}"):
		return strings.TrimSuffix(strings.TrimPrefix(ref, "{env:"), "}")
	default:
		return ref
	}
}

// SplitLegacyArgs splits a legacy free-form arguments string into structured args.
// It does not invoke a shell; it splits on whitespace.
func SplitLegacyArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.Fields(raw)
}

// SplitLegacyEnvRefs splits a comma/whitespace separated env reference list.
func SplitLegacyEnvRefs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
