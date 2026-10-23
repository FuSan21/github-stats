FROM node:22-alpine
WORKDIR /app
COPY server.mjs ./

# The views volume mounts here; owning it in the image gives a fresh named
# volume the same owner.
RUN mkdir /data && chown node:node /data
VOLUME /data

USER node
EXPOSE 7000
CMD ["node", "server.mjs"]
