# syntax=docker/dockerfile:1
FROM node:22-alpine

# Pinned upstream commit; bump deliberately.
ADD https://github.com/anuraghazra/github-readme-stats.git#54a7985aeefda00d5eadb55b80c17c7f976c37d2 /app
WORKDIR /app

# express is a devDependency upstream, so install everything. --ignore-scripts
# skips the husky "prepare" hook, which needs a git checkout.
RUN npm ci --ignore-scripts && npm cache clean --force

USER node
EXPOSE 9000
CMD ["node", "express.js"]
