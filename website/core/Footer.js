const React = require('react');

const siteConfig = require(process.cwd() + '/siteConfig.js');

class Footer extends React.Component {
  render() {
    const currentYear = new Date().getFullYear();
    return (
      <footer className="nav-footer" id="footer">

        <section className="copyright">
          {siteConfig.copyright}
        </section>
      </footer>
    );
  }
}

module.exports = Footer;
