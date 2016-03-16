PROJECT_NAME = continuedb

.PHONY: install
install:
	# TODO

# `test` runs all companion tests(which, if has, should be ended with `_test.go`).
.PHONY: test
test: proto
	@$(foreach file, $(shell find . -name "main_test.go"), \
		echo "Testing $(file)" && \
		go test -v $(PROJECT_NAME)/$(subst /,,$(subst ./,,$(dir $(file)))))

PROTOBUF_INCLUDE = $(GOPATH)/src/:./
PROTOBUF_SUFFIX = proto
PROTOBUF_HANDLER = protoc -I $(PROTOBUF_INCLUDE) --gofast_out=plugins=grpc:./ $(1)

# `proto` compiles all protobuf files in the project.
.PHONY: proto
proto:
	@echo "Compile protobuf files."
	@$(foreach file, $(shell find . -name "*.$(PROTOBUF_SUFFIX)"), \
		$(call PROTOBUF_HANDLER, $(file));)

.PHONY: clean
clean:
	@$(foreach file, $(shell find . -name "*.pb.go"), $(shell rm -f $(file)))
