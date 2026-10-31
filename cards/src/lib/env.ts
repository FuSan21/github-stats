// Read lazily so tests can set the environment before first use.
export const env = {
  get user() {
    return process.env.GITHUB_USER ?? "";
  },
  get token() {
    return process.env.GITHUB_TOKEN ?? "";
  },
  get port() {
    return Number(process.env.PORT || 7000);
  },
  get viewsFile() {
    return process.env.VIEWS_FILE || "/data/views.json";
  },
  get initialViews() {
    return Number(process.env.INITIAL_VIEWS || 0);
  },
};
