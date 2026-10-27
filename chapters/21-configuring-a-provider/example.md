# 21 Example

*The finished implementation, taken apart. Open this after `miyagi submit 21` passes.*

The file it describes is `reference/ch21/agent/config.go`.

---

## Resolving in passes

```go
func LoadConfig(scopes []ConfigScope, env map[string]string, flags FileConfig) (Config, error) {
	cfg := Config{Sources: map[string]string{}}

	for _, scope := range scopes {
		file, err := readConfigFile(scope.Path)
		if err != nil {
			return cfg, err
		}
		if file == nil {
			continue
		}
		applyFile(&cfg, *file, scope.Name)
	}

	applyEnv(&cfg, env)
	applyFile(&cfg, flags, "flag")

	if err := resolveKey(&cfg); err != nil {
		return cfg, err
	}
	applyDefaults(&cfg)
	return cfg, nil
}
```

Four passes, each overwriting the last: files, environment, flags, then defaults. Writing
it as passes rather than a merge function keeps the rule visible in the code, and makes
"why did this value win" answerable by reading one screen.

The same `applyFile` handles both file sources and flags. A flag is a config file with one
setting in it.

## Pointers make "unset" expressible

```go
type FileConfig struct {
	BaseURL *string `json:"base_url"`
	Model   *string `json:"model"`
	...
}
```

Without pointers, a `string` field cannot distinguish "the file says empty" from "the file
does not mention it". The first should clear a value; the second should let the next source
have its turn. `nil` is the only way to say "no opinion".

`applyFile` therefore checks every field:

```go
if file.BaseURL != nil {
	cfg.BaseURL, cfg.Sources["base_url"] = *file.BaseURL, source
}
```

## Disallowing unknown keys

```go
decoder := json.NewDecoder(strings.NewReader(string(body)))
decoder.DisallowUnknownFields()
```

A typo in a config file is otherwise invisible. `"modle": "gpt-4"` decodes without error,
sets nothing, and the run silently uses the default model. Failing on an unknown field turns
that into `agent.json: json: unknown field "modle"`, which is the whole message you need.

The cost is that an older binary rejects a config file written for a newer one, which is the
right trade for a tool people configure once and then forget.

## The key file

```go
key := strings.TrimSpace(string(body))
if key == "" {
	return fmt.Errorf("api_key_file: %s is empty", path)
}
cfg.APIKey = key
cfg.Sources["api_key"] = "file " + path
```

`TrimSpace` because editors add a trailing newline and `Bearer sk-...\n` is a 401 that looks
like a wrong key.

Expanding `~/` by hand rather than through `filepath` because a config file is written by a
person, and `~` is what a person writes.

An inline key is checked first:

```go
if cfg.APIKey != "" {
	return nil
}
```

So `OPENAI_API_KEY=... ./agent -p "hi"` overrides a file for one command, which is what you
want when testing a second account.

## Defaults last

```go
func applyDefaults(cfg *Config) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434/v1"
		cfg.Sources["base_url"] = "default"
	}
	...
}
```

Last, so that a default never overwrites a real value. The source is recorded as `default`
rather than left empty, so the report can distinguish "you set this" from "nobody set this".

`http://localhost:11434/v1` is Ollama's default address, which means a fresh clone works
against a local model with no configuration at all.

## Redaction

```go
redact := func(s string) string {
	if s == "" {
		return "(none)"
	}
	if len(s) <= 8 {
		return "(set)"
	}
	return s[:4] + "…" + s[len(s)-4:]
}
```

Four characters at each end is enough to tell two keys apart and not enough to use either.
Short keys are reported as `(set)` rather than shown, because a secret that fits in eight
characters is not a secret.

The report is the command people will paste into a support thread, so redacting is not
politeness, it is the difference between a useful report and a leaked key.

## Validating the URL

```go
if !strings.Contains(raw, "://") {
	return fmt.Errorf("base_url %q has no scheme, try https://%s", raw, raw)
}
if strings.HasSuffix(raw, "/chat/completions") {
	return fmt.Errorf("base_url %q includes the path; it should stop at /v1", raw)
}
```

Two mistakes, both common, neither producing an error that names the setting. A URL without
a scheme makes the HTTP client read `api.example.test/v1` as a path and fail with something
about protocols. A URL that already includes `/chat/completions` gets joined again and 404s.

This is a warning in `-config` rather than a hard failure in the run, because a proxy URL
might legitimately surprise the check and a person should still be able to start the agent.

## The fixes that matter

**1. Files read in the wrong order** (`flag-beats-file`)

```diff
- applyEnv(&cfg, env); applyFile(&cfg, files, "user")
+ // files, then environment, then flags
```

**2. Values instead of pointers** (`config-from-file`)

```diff
- type FileConfig struct { Model string }
+ type FileConfig struct { Model *string }
```

**3. Printing the key** (`config-report-redacts-the-key`)

```diff
- fmt.Sprintf("api_key  %s", c.APIKey)
+ fmt.Sprintf("api_key  %s", redact(c.APIKey))
```

**4. Checking config after needing a prompt** (`config-needs-no-provider`)

```diff
- if *prompt == "" { return error } ... if *showConfig { ... }
+ if *showConfig { ... } ... if *prompt == "" { return error }
```

## What to compare against your own

- [ ] files, then environment, then flags
- [ ] a field missing from a file does not clear an earlier value
- [ ] an unknown key is an error
- [ ] a missing file is not an error
- [ ] the key file is trimmed and can be overridden inline
- [ ] `-config` needs no provider and no prompt
- [ ] the report names sources and redacts the key

Everything outside that list is style.
