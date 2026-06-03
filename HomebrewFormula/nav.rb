class Nav < Formula
  desc "Tiny terminal file navigator with vim keys"
  homepage "https://github.com/TheGentleTurtle/nav"
  url "https://github.com/TheGentleTurtle/nav/archive/refs/tags/v1.3.4.tar.gz"
  sha256 "67a79fcdeb6a717273b5228ab27fdd2cc72618325b17bf279139e091918cc837"
  license "CC-BY-NC-4.0"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w")
  end

  test do
    assert_match "nav", shell_output("#{bin}/nav --help 2>&1")
  end
end
