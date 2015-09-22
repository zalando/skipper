package randpath_test

import (
	"fmt"
	"github.com/zalando/skipper/mock/randpath"
)

func ExamplePathGenerator() {
	rp := randpath.MakePathGenerator(randpath.PathGeneratorOptions{RandSeed: 42})
	fmt.Println(rp.Next())
	fmt.Println(rp.Next())

	// Output:
	// /ukptt/ezptneuvunhuks/vgzadxl/ghejkmvezjpmkqa/mtrgkfbswyju/gkdmd/wqgrwrqzwwhad/npbbcyvursmmnz/
	// /pauycozuuwbkj/ntjlmjdryzy/
}
