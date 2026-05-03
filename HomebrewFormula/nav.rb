class Nav < Formula
  desc "Tiny terminal file navigator with vim keys"
  homepage "https://github.com/TheGentleTurtle/nav"
  url "https://github.com/TheGentleTurtle/nav/archive/refs/tags/v1.1.4.tar.gz"
  sha256 "a4a56fe1c765cd6e60ddd010b9f8fe68afdbccefe52835e7edbcdec216cedc35"
  license "CC-BY-NC-4.0"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w")
  end

  test do
    assert_match "nav", shell_output("#{bin}/nav --help 2>&1")
  end
end
