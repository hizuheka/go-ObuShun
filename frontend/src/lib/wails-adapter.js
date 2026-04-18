/**
 * Wails Adapter for Shun
 * This file mocks Tauri APIs and bridges them to Wails Go backend.
 */

// Helper to check if Wails is ready
const isWailsReady = () => !!(window.go && window.go.main && window.go.main.App);

export const getCurrentWindow = () => ({
  setSize: async (size) => {
    if (isWailsReady() && window.go.main.App.SetWindowSize) {
      await window.go.main.App.SetWindowSize(size.width, size.height);
    }
  },
  startDragging: async () => {
    if (window.runtime && window.runtime.WindowStartDrag) {
      window.runtime.WindowStartDrag();
    }
  },
  listen: async () => {},
  onResized: async () => {},
  hide: async () => {
    if (isWailsReady() && window.go.main.App.HideWindow) {
      await window.go.main.App.HideWindow();
    }
  }
});

export class LogicalSize {
  constructor(w, h) {
    this.width = w;
    this.height = h;
  }
}

export const listen = async (eventName, callback) => {
  try {
    if (window.runtime && window.runtime.EventsOn) {
      window.runtime.EventsOn(eventName, (...args) => {
        console.log(`Received event: ${eventName}`, args);
        callback({ payload: args.length > 0 ? args[0] : null });
      });
    } else {
      // Retry if runtime is not yet injected
      setTimeout(() => listen(eventName, callback), 100);
    }
  } catch (e) {
    console.warn("Failed to register listen:", eventName, e);
  }
};

export const invoke = async (cmd, args) => {
  console.log("Invoke:", cmd, args);
  
  // Wait for Wails to be ready for critical commands
  if (!isWailsReady()) {
    // For search/config, wait a bit
    await new Promise(resolve => setTimeout(resolve, 50));
    if (!isWailsReady()) return null;
  }

  const app = window.go.main.App;

  switch (cmd) {
    case "get_config_and_warnings":
      if (app.GetConfigAndWarnings) {
        const [config, warnings] = await app.GetConfigAndWarnings();
        return { config, warnings };
      }
      return { config: {}, warnings: [] };

    case "list_config_files":
      if (app.ListConfigFiles) return await app.ListConfigFiles();
      return ["config.toml"];

    case "search_items":
      if (app.SearchItems) {
        return await app.SearchItems(args.query || "", args.searchMode || "", args.sortOrder || "");
      }
      return [];

    case "launch_item":
      if (app.LaunchItem) {
        return await app.LaunchItem(args.item, args.extraArgs || []);
      }
      return null;

    case "complete_path":
      // Currently partially implemented in Go if needed, or return empty for now
      return { completions: [], prefix: "" };

    default:
      console.warn(`Unknown command: ${cmd}`);
      return null;
  }
};
