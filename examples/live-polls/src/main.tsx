import React from "react";
import ReactDOM from "react-dom/client";
import { AYBProvider, type AYBClientLike } from "@allyourbase/react";
import App from "./App";
import { ayb } from "./lib/ayb";
import "./index.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AYBProvider client={ayb as unknown as AYBClientLike}>
      <App />
    </AYBProvider>
  </React.StrictMode>,
);
