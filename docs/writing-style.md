# Writing style

Every page in this repository follows these rules. They come from the Queryassay
house style, which applies three published standards together: Diátaxis for
structure, Vercel's writing guidelines for sentences, and the `unslop` catalogue
for AI tells. The rules below are the operative subset and stand on their own.

## Sentences

- Keep a sentence under 25 words. One idea per sentence.
- Use the active voice and the present tense. Append "by monkeys" to a sentence;
  if it still parses, it is passive, so rewrite it.
- Address the reader as `you`. Use the imperative for a step.
- Use the short everyday word: use, help, do. Not utilize, facilitate, perform.
- Cut every word that does no work. "In order to" is "to".
- Name the real symbol, file, flag, or command. The codebase is the word list.
- Do not use an em dash. Use a comma, a colon, or a new sentence.
- Do not say "is" the long way. "Serves as", "stands as", and "boasts" are "is".
- Do not write "not just X, but Y". State the point.
- Do not force ideas into groups of three. Do not cycle synonyms for one thing.
  Pick one term and repeat it.
- Do not open a section with a slogan. Do not restate the heading.
- State what a thing does, then point at the file that proves it.
- Prefer a measured number, or a test, over an adjective.

## Vocabulary

Do not use: additionally, crucial, delve, leverage, robust, seamless, showcase,
underscore, vibrant, comprehensive, powerful, effortless, unlock, elevate,
landscape, realm, journey, testamount, pivotal, meticulous, intricate,
ever-evolving, game-changer, best-in-class, cutting-edge, paradigm, synergy,
streamline, harness (as a verb), utilize, numerous, myriads.

## Structure

- No emoji. No title case in headings. No rhetorical questions.
- A table only when the reader compares values across rows.
- Do not start a list item with a bold label and a dash.
- Say what is unfinished in the same document, in its own section.
- Depth comes from explaining the mechanism and the consequence, not from adding
  edge cases or repeating the point in new words.

## For this course specifically

- `overview.md` explains the mechanism. It may not contain a working
  implementation.
- `problem.md` states the task and the graded claims. It may not teach the
  concept; it points at the overview for that.
- `example.md` shows a full annotated implementation. It may not introduce a
  concept that the overview did not already cover.
- Every claim in `problem.md` names the case id that grades it. If a claim has no
  case, either the claim is prose or the case is missing. Both are bugs.
- A trap in the prose names the case that catches it. That link is what stops the
  teaching and the grading from drifting apart.
