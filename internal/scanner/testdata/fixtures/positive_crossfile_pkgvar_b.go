package fixtures

func useCrossFilePkgVar() {
	_, _ = crossFilePkgClient.StringVariation("crossfile-pkgvar-flag", nil, "default")
}
