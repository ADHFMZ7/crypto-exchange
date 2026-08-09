import React from "react";
import { SourceBadge } from "./DataSource";
import { ENDPOINTS, LIVE_ENDPOINT_COUNT } from "../lib/endpoints";

/**
 * Renders the frontend/backend contract from lib/endpoints so the integration
 * gap is visible in the running app.
 */
export const IntegrationStatus: React.FC = () => (
  <section className="panel">
    <div className="headline">
      <div>
        <div className="tag">Integration</div>
        <h2 className="panel-title">Backend coverage</h2>
      </div>
      <div className="pill">
        <strong>
          {LIVE_ENDPOINT_COUNT}/{ENDPOINTS.length}
        </strong>{" "}
        <span className="muted">endpoints live</span>
      </div>
    </div>

    <table className="table">
      <thead>
        <tr>
          <th>Endpoint</th>
          <th>Purpose</th>
          <th>Status</th>
        </tr>
      </thead>
      <tbody>
        {ENDPOINTS.map((endpoint) => (
          <tr key={`${endpoint.method} ${endpoint.path}`}>
            <td>
              <code>
                {endpoint.method} {endpoint.path}
              </code>
            </td>
            <td>
              {endpoint.purpose}
              {endpoint.workaround && (
                <div className="muted" style={{ fontSize: 13 }}>
                  Meanwhile: {endpoint.workaround}
                </div>
              )}
            </td>
            <td>
              <SourceBadge kind={endpoint.state === "live" ? "live" : "mock"} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  </section>
);
