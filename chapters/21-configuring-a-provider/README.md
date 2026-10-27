# 21 Configuring a provider

**Part VII · Store** · Difficulty: medium · 45 to 60 minutes · 9 graded cases

## What you are building

A way to say which provider to use, once, and a command that tells you what it resolved.

```sh
$ cat agent.json
{
  "base_url": "https://api.openai.com/v1",
  "model": "gpt-4o-mini",
  "api_key_file": "~/.config/kata/openai.key"
}

$ ./agent -config
base_url      https://api.openai.com/v1               (project)
model         gpt-4o-mini                             (project)
api_key       sk-a…c9f1                               (file /home/you/.config/kata/openai.key)
max_steps     20
max_context   8000
max_tools     6
temperature   0

$ ./agent -p "hello"
```

Three sources, one resolved set of settings, and a report that says where each value came
from.

You edit `agent/cmd/agent/main.go`. You also create one new file, `agent/config.go`, holding the two config types, the loader and `Describe`.
## Three sources, in order

```
  agent.json in ~/.config/kata      lowest priority
  agent.json in the project
  environment variables
  flags                              highest priority
```

Later wins. The order follows the lifetime of each thing: a file is shared and long-lived,
an environment variable belongs to a shell, and a flag belongs to one command.

This is the same precedence rule as the skill scopes, and for the same reason: the more
specific, shorter-lived thing should win.

## Pointers, not values

```go
type FileConfig struct {
	BaseURL *string `json:"base_url"`
	Model   *string `json:"model"`
	// ...
}
```

A field of type `string` cannot distinguish "the file says model is empty" from "the file
does not mention model". With pointers, `nil` means the file was silent and the next source
gets a turn.

The same `FileConfig` type carries the flags, so both paths apply settings identically.

## The key belongs in a file

An environment variable is readable in a process listing, lands in shell history, and is
inherited by every child process. A file can be `chmod 600` and stays out of both.

```json
{"api_key_file": "~/.config/kata/openai.key"}
```

An inline `api_key` still wins, so a shell can override a file for one session.

```
  api_key         set inline           wins
  api_key_file    read, trimmed        used when no inline key
  neither         no key sent          allowed: a local model needs none
```

## Two mistakes worth catching

```
  api.example.test/v1        no scheme: the client treats the host as a path
  https://x/v1/chat/completions   the path is already included
```

Neither produces an error that names the setting. The first gives a protocol error, the
second a 404 at the wrong path. `-config` says which one you have.

## Unknown keys are an error

```go
decoder.DisallowUnknownFields()
```

A typo in a config file is otherwise silent: `"modle": "gpt-4"` is accepted, ignored, and
the agent uses the default. Failing on an unknown key turns a shrug into a message.

## What you need to build

```go
type Config struct {
    BaseURL, Model, APIKey, KeyFile string
    MaxSteps, MaxContext, MaxTools  int
    Temperature                     float64
    Sources                         map[string]string
}

type FileConfig struct {
    BaseURL     *string  `json:"base_url"`
    Model       *string  `json:"model"`
    APIKey      *string  `json:"api_key"`
    KeyFile     *string  `json:"api_key_file"`
    MaxSteps    *int     `json:"max_steps"`
    MaxContext  *int     `json:"max_context"`
    MaxTools    *int     `json:"max_tools"`
    Temperature *float64 `json:"temperature"`
}

func DefaultConfigScopes(projectRoot string) []ConfigScope
func LoadConfig(scopes []ConfigScope, env map[string]string, flags FileConfig) (Config, error)
func ValidateBaseURL(raw string) error
func (c Config) Describe() []string
```

Three flags:

```sh
agent -config                 print the resolved settings and exit
agent -base-url <url>         override the endpoint for this run
agent -api-key-file <path>    override the key file for this run
```

## How you are checked

The verifier builds your program, writes config files into a temporary workspace and a
temporary home directory, and runs it with a scripted endpoint.

- `config-from-file`: `agent.json` supplies the endpoint and model, and the request uses
  them.
- `flag-beats-file`: `-base-url` and `-m` override the file.
- `all-three-sources-at-once`: with a file, an environment variable and a flag all set, the
  flag wins; without the flag, the environment still beats the file.
- `env-beats-file`: `OPENAI_BASE_URL` and `HARNESS_MODEL` override the file.
- `key-from-file`: `api_key_file` supplies the bearer token.
- `missing-key-file-is-reported`: a missing key file exits non-zero, names the path, and
  makes no request.
- `config-report-redacts-the-key`: the report shows the URL, the model, and the source, and
  never the key.
- `config-needs-no-provider`: `-config` works with a dead endpoint.
- `bad-base-url-is-explained`: a URL with no scheme produces a warning naming the problem.

## Notes

`-config` is read-only. Handle it before the prompt check, for the same reason `-usage` and
`-sessions` are.

`HOME` and `XDG_CONFIG_HOME` decide where the user file lives. The checks set both to a
temporary directory so your real settings cannot change the result.

A file that does not exist is not an error. That is the normal state of a new project.

## If you get stuck

<details><summary>First hint: the shape of the problem</summary>

Either a source is read in the wrong order, a missing file is treated as an error, or the
report prints something it should not.
</details>

<details><summary>Second hint: what you will need</summary>

`json.NewDecoder` with `DisallowUnknownFields`, `os.UserHomeDir`, `os.Getenv`
`XDG_CONFIG_HOME`, and `strings.TrimSpace` on a key file.
</details>

<details><summary>Third hint: the finished loader</summary>

`LoadConfig` walks the scopes lowest first, decoding each existing file into `FileConfig` and
applying only the fields that are not nil. Then it applies the environment, then the flag
struct. `resolveKey` reads the key file when no inline key was given. `applyDefaults` fills
the endpoint, model and numeric limits. `Describe` prints each value with its source, and the
key as four characters, an ellipsis, and four characters.

`kata hint 21 <case-id>` prints the same three hints.
</details>

## Done when

`kata submit 21` reports 9/9, and `./agent -config` tells you where every setting came from.

[example.md](example.md) has the finished code, after you pass.
