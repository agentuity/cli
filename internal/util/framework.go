package util

// these are the keys that will be written in order to the package.json file
// if a key is not present, it will be added to the end of the file
var PackageJsonKeysOrder = []string{
	"name",
	"description",
	"version",
	"main",
	"type",
	"scripts",
	"keywords",
	"author",
	"license",
	"engines",
	"private",
	"devDependencies",
	"peerDependencies",
	"dependencies",
}

// these are the keys that will be written in order to the package.json file
// if a key is not present, it will be added to the end of the file
var TypeScriptConfigJsonKeysOrder = []string{
	"compilerOptions",
	"include",
	"exclude",
}
