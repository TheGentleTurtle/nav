class Nav < Formula
  desc "Tiny terminal file navigator with vim keys"
  homepage "https://github.com/TheGentleTurtle/nav"
  url "https://github.com/TheGentleTurtle/nav/archive/refs/tags/v1.1.7.tar.gz"
  sha256 "9f29a546d0a40240017e7267ae1c5c86ef795b2b750c42a4d8b31c1c0c95c1b8"
  license "CC-BY-NC-4.0"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w")
  end

  test do
    assert_match "nav", shell_output("#{bin}/nav --help 2>&1")
  end
end
