// Reference solutions are intentionally a separate module so that the root
// module never compiles them. They are file-level replacements for
// harness/*.go and only build after `craft test --solution` overlays them.
module kata/reference

go 1.26
