FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
# Static binary: no libc, so the runtime image can be empty. Zone data is
# compiled in (time/tzdata), and themes are embedded.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /cards . \
 && mkdir /data

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /cards /cards
# The views volume mounts here; owning it in the image gives a fresh named
# volume the same owner. uid 1000 matches the earlier node and bun images, so
# an existing volume stays writable.
COPY --from=build --chown=1000:1000 /data /data
VOLUME /data
USER 1000:1000
EXPOSE 7000
ENTRYPOINT ["/cards"]
