// The reference implementations are a separate module so the root module never
// compiles them. Each reference/chNN holds only the files chapter NN changes, and
// `kata submit NN --solution` overlays chapters 01 through NN into a throwaway
// copy of the repository before grading.
module agent/reference

go 1.26
