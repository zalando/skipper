package mock_test

import (
	"fmt"
	"github.com/zalando/skipper/mock"
)

func ExamplePathGenerator() {
	rp := mock.MakePathGenerator(mock.PathGeneratorOptions{RandSeed: 42})
	fmt.Println(rp.Next())
	fmt.Println(rp.Next())

	// Output:
	// /ukptt/ezptneuvunhuks/vgzadxl/ghejkmvezjpmkqa/mtrgkfbswyju/gkdmd/wqgrwrqzwwhad/npbbcyvursmmnz/
	// /pauycozuuwbkj/ntjlmjdryzy/
}
