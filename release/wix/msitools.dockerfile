FROM public.ecr.aws/docker/library/alpine:3.24.1
RUN apk update && apk add jq msitools
WORKDIR /workspace