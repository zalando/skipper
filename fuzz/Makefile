MKDIR			= mkdir -p
GO_FUZZ_BUILD		?= go114-fuzz-build

PREFIX			?= $(CURDIR)
ARTIFACTS_PATH		= $(PREFIX)/artifacts
DICTIONARY_PATH		= $(PREFIX)/dictionaries
CORPUS_PATH		= $(PREFIX)/corpus
FUZZ_TARGETS_PATH	= $(PREFIX)/fuzz_targets

FUZZ_TARGETS 		?= FuzzParseCIDRs FuzzParseEskip FuzzParseFilters \
				FuzzParseIngressV1JSON FuzzParseIPCIDRs \
				FuzzParseJwt FuzzParsePredicates \
				FuzzParseRouteGroupsJSON FuzzServer

FUZZ_SANITIZER		?= address
FUZZ_FORKS		?= 2
FUZZ_MAX_TOTAL_TIME	?= 600
FUZZ_RUNS		?= -1
FUZZER_ARGS		?= -artifact_prefix=$(ARTIFACTS_PATH) \
				-rss_limit_mb=2560 -timeout=25 \
				-max_total_time=$(FUZZ_MAX_TOTAL_TIME) \
				-len_control=0 -detect_leaks=0 -max_len=4096 \
				-fork=$(FUZZ_FORKS) -runs=$(FUZZ_RUNS)

ifeq ($(FUZZ_SANITIZER), address)
	FUZZ_BUILD_ARGS = -fsanitize=fuzzer -fsanitize=address -fsanitize-address-use-after-scope
else ifeq ($(FUZZ_SANITIZER), undefined)
	FUZZ_BUILD_ARGS = -fsanitize=fuzzer -fsanitize=array-bounds,bool,builtin,enum,float-divide-by-zero,function,integer-divide-by-zero,null,object-size,return,returns-nonnull-attribute,shift,signed-integer-overflow,unsigned-integer-overflow,unreachable,vla-bound,vptr -fno-sanitize-recover=array-bounds,bool,builtin,enum,float-divide-by-zero,function,integer-divide-by-zero,null,object-size,return,returns-nonnull-attribute,shift,signed-integer-overflow,unreachable,vla-bound,vptr
else ifeq ($(FUZZ_SANITIZER), none)
	FUZZ_BUILD_ARGS = -fsanitize=fuzzer
else
$(error $(FUZZ_SANITIZER) is invalid sanitizer, available options are address, undefined, none)
endif

export ASAN_OPTIONS	= alloc_dealloc_mismatch=0:allocator_may_return_null=1:allocator_release_to_os_interval_ms=500:check_malloc_usable_size=0:detect_container_overflow=1:detect_odr_violation=0:detect_leaks=1:detect_stack_use_after_return=1:fast_unwind_on_fatal=0:handle_abort=1:handle_segv=1:handle_sigill=1:max_uar_stack_size_log=16:print_scariness=1:quarantine_size_mb=10:strict_memcmp=1:symbolize=1:use_sigaltstack=1:dedup_token_length=3
export UBSAN_OPTIONS	= print_stacktrace=1:print_summary=1:silence_unsigned_overflow=1:symbolize=1:dedup_token_length=3

.PHONY: help all fuzz clean
default: all

help: ## display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

all: $(FUZZ_TARGETS:=.out) ## build all fuzz binaries

%.a:
	$(GO_FUZZ_BUILD) -func $(@:.a=) -o $@ $(FUZZ_TARGETS_PATH)

gosigfuzz/gosigfuzz.o:
	$(CC) -c $(@:.o=.c) -o $@

%.out: %.a gosigfuzz/gosigfuzz.o
	$(CC) $(FUZZ_BUILD_ARGS) -lresolv $^ -o $@

fuzz: $(FUZZ_TARGETS:=.out) ## run all fuzz tests
	for TARGET in $(FUZZ_TARGETS); do \
		$(MKDIR) $(CORPUS_PATH)/$$TARGET $(ARTIFACTS_PATH); \
		ARGS="$(FUZZER_ARGS)"; \
		[ -f "$(DICTIONARY_PATH)/$$TARGET.dict" ] && ARGS="$$ARGS -dict=$(DICTIONARY_PATH)/$$TARGET.dict"; \
		$(PREFIX)/$$TARGET.out $$ARGS $(CORPUS_PATH)/$$TARGET; \
	done

clean: ## clean temporary files and directories
	$(RM) $(PREFIX)/*.out $(PREFIX)/*.a $(PREFIX)/*.h $(PREFIX)/main.*.go gosigfuzz/gosigfuzz.o
