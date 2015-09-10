#############
# Orchestra #
#############

PACKAGES := conductor player submitjob getstatus
LIBS     := orchestra
DEPS     := \
	github.com/kuroneko/configureit \
	github.com/golang/protobuf/proto

########################################################################

all: $(PACKAGES:%=bin/%)

GO_TOOLS := fmt vet

# Avoid errors during tab-completion
ifdef __BASH_MAKE_COMPLETION__
$(PACKAGES:%=bin/%):
else

define bin-prereq
$(1)          ?= $(2)
sh-check-$(1) := command -v '$(2)' >/dev/null 2>/dev/null
have-$(1)     := $$(shell $$(sh-check-$(1)) && echo 1)
need-$(1)::
ifdef have-$(1)
ifdef V
$$(info # Found $(1) = $(2))
endif
else
need-$(1)::
	@echo "'$(2)' is not in your PATH" >&2
	@echo "(Hint: try passing a $(1)= variable)" >&2
	@echo >&2
	@exit 1
endif

endef

$(eval $(call bin-prereq,GO,go))
$(eval $(call bin-prereq,PROTOC,protoc))
$(eval $(call bin-prereq,PROTOC_GEN_GO,protoc-gen-go))

define pkg-prereq
ok := $$(shell $(GO) list -f '' '$(1)' 2>/dev/null && echo 1)
ifdef ok
ifdef V
$$(info # Found $(1))
endif
else
need-GO::
	@echo "'$(1)' is not in your GOPATH" >&2
	@echo "(Hint: try installing it with '$(GO) get $(1)')" >&2
	@echo >&2
	@exit 1
endif

endef

$(eval $(foreach _,$(DEPS),$(call pkg-prereq,$(_))))

GO_HERE := GOPATH=$(CURDIR)$(if $(GOPATH),:)$(GOPATH) $(GO)
GOOS    ?= $(shell $(GO) env GOOS   2>/dev/null)
GOARCH  ?= $(shell $(GO) env GOARCH 2>/dev/null)

$(if $(GOOS),$(if $(V),$(info # Set GOOS = $(GOOS))))
$(if $(GOARCH),$(if $(V),$(info # Set GOARCH = $(GOARCH))))

GO_FLAGS :=$(if $(GOCCFLAGS), -ccflags '$(GOCCFLAGS)')$(if $(GOGCFLAGS), -gcflags '$(GOGCFLAGS)')$(if $(GOLDFLAGS), -ldflags '$(GOLDFLAGS)')

define build-bin
bin/$(1): src/$(1)/*.go $(LIBS:%=pkg/$(GOOS)_$(GOARCH)/%.a) | need-GO
	$$(GO_HERE) build$(GO_FLAGS) -o $$@ $(1)

endef

define build-pkg
pkg/$(GOOS)_$(GOARCH)/$(1).a: src/$(1)/*.go | need-GO
	$$(GO_HERE) build$(GO_FLAGS) -o $$@ $(1)

endef

$(eval $(foreach _,$(PACKAGES),$(call build-bin,$(_))))
$(eval $(foreach _,$(LIBS),$(call build-pkg,$(_))))

pkg/$(GOOS)_$(GOARCH)/orchestra.a: src/orchestra/orchestra.pb.go
%.pb.go: %.proto | need-PROTOC need-PROTOC_GEN_GO
	$(PROTOC) --go_out=. $<

$(GO_TOOLS): | need-GO
	$(GO_HERE) $@ $(PACKAGES) $(LIBS)

ifdef V
nil :=
$(info $(nil))
endif

endif

########################################################################

GIT            ?= git
ARCHIVE_FORMAT ?= tar.gz
ARCHIVE_COMMIT ?= HEAD

archive:
	$(GIT) archive --format=$(ARCHIVE_FORMAT) --prefix=orchestra-$$($(GIT) rev-parse $(ARCHIVE_COMMIT))/ --output=orchestra-$$($(GIT) describe --tags --always $(ARCHIVE_COMMIT)).tar.gz $(ARCHIVE_COMMIT)

########################################################################

clean:
	-rm -rf bin pkg src/orchestra/orchestra.pb.go orchestra-*.tar.gz

.PHONY: all clean $(GO_TOOLS)
