# syntax=docker/dockerfile:1
FROM node:22-alpine AS build

# Pinned upstream commit; bump deliberately.
ADD https://github.com/Ashutosh00710/github-readme-activity-graph.git#0962e9461de815933d462e8b98c7ce566cada8fb /app
WORKDIR /app
RUN npm ci --ignore-scripts && npm run build && npm prune --omit=dev

FROM node:22-alpine
WORKDIR /app
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/dist ./dist
COPY --from=build /app/package.json ./

USER node
EXPOSE 5100
CMD ["node", "dist/main.js"]
