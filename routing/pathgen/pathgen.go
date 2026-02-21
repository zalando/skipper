// Package pathgen is a test infrastructure package to support tests
// in the routing module.
package pathgen

import (
	"math/rand/v2"
	"strings"
)

const (
	defaultChars                = "abcdefghijklmnopqrstuvwxyz"
	defaultMinFilenameLength    = 3
	defaultMaxFilenameLength    = 18
	defaultMinNamesInPath       = 0
	defaultMaxNamesInPath       = 9
	defaultClosingSlashInEveryN = 3
	defaultSeparator            = "/"
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

// PathGenerator generates paths, separated with a slash or custom separator.
// The paths have a random number of filenames in them, and the
// filenames consist of random characters of random length.
// The generated sequences are reproducible, controlled by
// the RandSeed option.
type PathGenerator struct {
	options *PathGeneratorOptions
	Rnd     *rand.Rand
}

func applyDefaults(o *PathGeneratorOptions) {
	if o.FilenameChars == "" {
		o.FilenameChars = defaultChars
	}

	if o.MinFilenameLength == 0 {
		o.MinFilenameLength = defaultMinFilenameLength
	}

	if o.MaxFilenameLength == 0 {
		o.MaxFilenameLength = defaultMaxFilenameLength
	}

	if o.MinNamesInPath == 0 {
		o.MinNamesInPath = defaultMinNamesInPath
	}

	if o.MaxNamesInPath == 0 {
		o.MaxNamesInPath = defaultMaxNamesInPath
	}

	if o.ClosingSlashInEveryN == 0 {
		o.ClosingSlashInEveryN = defaultClosingSlashInEveryN
	}

	if o.Separator == "" {
		o.Separator = defaultSeparator
	}
}

// New creates a path generator with the provided options,
// falling back to the default value for each non-specified
// option field.
func New(o PathGeneratorOptions) *PathGenerator {
	var (
		// #nosec
		randSrc = rand.NewPCG(uint64(o.RandSeed), 0)
		randGen = rand.New(randSrc)
	)

	// options taken as value, free to modify
	applyDefaults(&o)

	return &PathGenerator{&o, randGen}
}

// Between takes a random number positioned between [min, max)
func (pg *PathGenerator) Between(min, max int) int {
	return min + pg.Rnd.IntN(max-min) // #nosec
}

// takes a random byte from the range of available characters
func (pg *PathGenerator) char() byte {
	return []byte(pg.options.FilenameChars)[pg.Rnd.IntN(len(pg.options.FilenameChars))] // #nosec
}

func (pg *PathGenerator) Str(min, max int) string {
	len := pg.Between(min, max)
	s := make([]byte, len)
	for i := range len {
		s[i] = pg.char()
	}

	return string(s)
}

func (pg *PathGenerator) Strs(min, max, minLength, maxLength int) []string {
	len := pg.Between(min, max)
	s := make([]string, len)
	for i := range len {
		s[i] = pg.Str(minLength, maxLength)
	}

	return s
}

// Name generates a random name using the available characters and of length within
// the defined boundaries
func (pg *PathGenerator) Name() string {
	return pg.Str(pg.options.MinFilenameLength, pg.options.MaxFilenameLength)
}

// Names generates random names of count between the defined boundaries
func (pg *PathGenerator) Names() []string {
	len := pg.Between(pg.options.MinNamesInPath, pg.options.MaxNamesInPath)
	names := make([]string, len)
	for i := range len {
		names[i] = pg.Name()
	}

	return names
}

// tells if using a closing slash for a path, based on the defined chance
func (pg *PathGenerator) closingSlash() bool {
	return pg.Rnd.IntN(pg.options.ClosingSlashInEveryN) == 0 // #nosec
}

// Next generates a random path.
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
func (pg *PathGenerator) Next() string {
	names := pg.Names()

	// appending an empty filename if a closing slash needs to be
	// added
	if pg.closingSlash() || len(names) == 0 {
		names = append(names, "")
	}

	// ensuring the path is absolute, prepending an empty filename
	names = append([]string{""}, names...)

	return strings.Join(names, pg.options.Separator)
}
