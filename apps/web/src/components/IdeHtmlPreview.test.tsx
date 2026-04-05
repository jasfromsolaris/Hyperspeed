import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { IdeHtmlPreview } from "./IdeHtmlPreview";

describe("IdeHtmlPreview", () => {
  it("renders nothing when not visible", () => {
    const { container } = render(<IdeHtmlPreview html="<p>x</p>" visible={false} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders server loading state", () => {
    render(
      <IdeHtmlPreview
        html=""
        visible
        mode="server"
        serverLoading
        serverUrl={null}
      />,
    );
    expect(screen.getByText(/starting preview/i)).toBeInTheDocument();
  });
});
