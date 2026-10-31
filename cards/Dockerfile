FROM oven/bun:1.4-alpine

# Zone data for TZ (the streak card's "today" and date labels).
RUN apk add --no-cache tzdata

WORKDIR /app
# No runtime dependencies: Bun runs the TypeScript directly.
COPY package.json ./
COPY src ./src

# The views volume mounts here; owning it in the image gives a fresh named
# volume the same owner. bun is uid 1000, like node in the old extras image,
# so an existing volume stays writable.
RUN mkdir /data && chown bun:bun /data
VOLUME /data

USER bun
EXPOSE 7000
CMD ["bun", "src/server.ts"]
