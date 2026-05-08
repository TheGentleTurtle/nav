class Nav < Formula
  desc "Tiny terminal file navigator with vim keys"
  homepage "https://github.com/TheGentleTurtle/nav"
  url "https://github.com/TheGentleTurtle/nav/archive/refs/tags/v1.3.1.tar.gz"
  sha256 "54e6c52a7124cef0c559926a60b8ae96ff5ba71f5f3eae826cffa5f77b50fb68"
  license "CC-BY-NC-4.0"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w")
  end

  test do
    assert_match "nav", shell_output("#{bin}/nav --help 2>&1")
  end
end
