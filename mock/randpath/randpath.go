package randpath

import (
	"math/rand"
	"strings"
)

const (
	DefaultChars                = "abcdefghijklmnopqrstuvwxyz"
	DefaultMinFilenameLength    = 3
	DefaultMaxFilenameLength    = 18
	DefaultMinNamesInPath       = 0
	DefaultMaxNamesInPath       = 9
	DefaultClosingSlashInEveryN = 3
	DefaultSeparator            = "/"
)

type PathGeneratorOptions struct {
	FilenameChars        string
	MinFilenameLength    int
	MaxFilenameLength    int
	MinNamesInPath       int
	MaxNamesInPath       int
	ClosingSlashInEveryN int
	RandSeed             int64
	Separator            string
}

// Generates paths, separated with a slash or custom separator.
// The paths have a random number of filenames in them, and the
// filenames consist of random characters of random length.
// The generated sequences are reproducible, controlled by
// the RandSeed option.
type PathGenerator interface {
	// Generates a random path.
	//
	// The path will be always absolute.
	//
	// The path may contain a closing slash, with a probability based on the
	// `ClosingSlashInEveryN`. If `ClosingSlashInEveryN < 0`, the path won't
	// contain a closing slash. If `ClosingSlashInEveryN == 0`, the path
	// will always contain a closing slash. If `ClosingSlashInEveryN == n`,
	// where `n > 0`, then the generated path will contain a closing slash
	// with a chance of `1 / n`.
	//
	// The path will contain a random number of names (the thing between the
	// slashes), equally distributed between `MinNamesInPath` and
	// `MaxNamesInPath`.
	//
	// The names in the path will have a random length, equally distributed
	// between `MinFilenameLength` and `MaxFilenameLength`.
	//
	// The sequence followed by `Next` is reproducible, to get a different
	// sequence, a new PathGenerator instance is required, with a
	// different `RandSeed` value.
	Next() string
}

type pathGenerator struct {
	options *PathGeneratorOptions
	rnd     *rand.Rand
}

func applyDefaults(o *PathGeneratorOptions) {
	if o.FilenameChars == "" {
		o.FilenameChars = DefaultChars
	}

	if o.MinFilenameLength == 0 {
		o.MinFilenameLength = DefaultMinFilenameLength
	}

	if o.MaxFilenameLength == 0 {
		o.MaxFilenameLength = DefaultMaxFilenameLength
	}

	if o.MinNamesInPath == 0 {
		o.MinNamesInPath = DefaultMinNamesInPath
	}

	if o.MaxNamesInPath == 0 {
		o.MaxNamesInPath = DefaultMaxNamesInPath
	}

	if o.ClosingSlashInEveryN == 0 {
		o.ClosingSlashInEveryN = DefaultClosingSlashInEveryN
	}

	if o.Separator == "" {
		o.Separator = DefaultSeparator
	}
}

// Creates a path generator with the provided options,
// falling back to the default value for each non-specified
// option field.
func MakePathGenerator(o PathGeneratorOptions) PathGenerator {

	// options taken as value, free to modify
	applyDefaults(&o)

	return &pathGenerator{&o, rand.New(rand.NewSource(o.RandSeed))}
}

// takes a random number positioned between [min, max)
func (pg *pathGenerator) between(min, max int) int {
	return min + pg.rnd.Intn(max-min)
}

// takes a random byte from the range of available characters
func (pg *pathGenerator) char() byte {
	return []byte(pg.options.FilenameChars)[pg.rnd.Intn(len(pg.options.FilenameChars))]
}

// generates a random name using the available characters and of length within
// the defined boundaries
func (pg *pathGenerator) name() string {
	len := pg.between(pg.options.MinFilenameLength, pg.options.MaxFilenameLength)

	name := make([]byte, len)
	for i := 0; i < len; i++ {
		name[i] = pg.char()
	}

	return string(name)
}

// generates random names of count between the defined boundaries
func (pg *pathGenerator) names() []string {
	len := pg.between(pg.options.MinNamesInPath, pg.options.MaxNamesInPath)
	names := make([]string, len)
	for i := 0; i < len; i++ {
		names[i] = pg.name()
	}

	return names
}

// tells if using a closing slash for a path, based on the defined chance
func (pg *pathGenerator) closingSlash() bool {
	return pg.rnd.Intn(pg.options.ClosingSlashInEveryN) == 0
}

func (pg *pathGenerator) Next() string {
	names := pg.names()

	// appending an empty filename in case a closing slash needs to be
	// added
	if pg.closingSlash() || len(names) == 0 {
		names = append(names, "")
	}

	// ensuring the path is absolute, prepending an empty filename
	names = append([]string{""}, names...)

	return strings.Join(names, pg.options.Separator)
}
