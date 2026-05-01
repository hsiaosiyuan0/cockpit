import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { reportFrontendError } from "./api";
import "./style.css";

class RootErrorBoundary extends React.Component<React.PropsWithChildren, { message: string }> {
  state = { message: "" };

  static getDerivedStateFromError(error: unknown) {
    return { message: error instanceof Error ? error.message : "Frontend render failed" };
  }

  componentDidCatch(error: unknown, info: React.ErrorInfo) {
    const message = error instanceof Error ? error.message : "Frontend render failed";
    const stack = [error instanceof Error ? error.stack : "", info.componentStack].filter(Boolean).join("\n");
    void reportFrontendError(message, stack);
  }

  render() {
    if (this.state.message) {
      return (
        <div className="fatal-screen">
          <div className="fatal-card">
            <span>COCKPIT RENDER FAULT</span>
            <strong>{this.state.message}</strong>
            <button type="button" onClick={() => window.location.reload()}>
              Reload
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

function installFrontendErrorLogging() {
  window.addEventListener("error", (event) => {
    const stack = event.error instanceof Error ? event.error.stack ?? "" : "";
    void reportFrontendError(event.message || "frontend error", stack);
  });
  window.addEventListener("unhandledrejection", (event) => {
    const reason = event.reason;
    const message = reason instanceof Error ? reason.message : String(reason || "unhandled promise rejection");
    const stack = reason instanceof Error ? reason.stack ?? "" : "";
    void reportFrontendError(message, stack);
  });
}

installFrontendErrorLogging();

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <RootErrorBoundary>
      <App />
    </RootErrorBoundary>
  </React.StrictMode>,
);
