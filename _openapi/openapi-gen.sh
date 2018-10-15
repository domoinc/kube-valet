# openapi-gen
go install ./$(dirname "${0}")/cmd/openapi-gen
if [ "${GENS}" = "all" ] || grep -qw "openapi" <<<"${GENS}"; then
  echo "Generating openapi spec for ${GROUPS_WITH_VERSIONS} at ${OUTPUT_PKG}/openapi"

  ${GOPATH}/bin/openapi-gen \
           --input-dirs $(codegen::join , "${FQ_APIS[@]}") \
           --output-package ${OUTPUT_PKG}/openapi \
           "$@"
fi