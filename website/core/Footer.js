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
        <section className="footer-logo">
          <a href={this.props.config.baseUrl} className="nav-home">
          {this.props.config.footerIcon && (
            <img
            src={this.props.config.baseUrl + this.props.config.footerIcon}
            alt={this.props.config.title}
            width="75"
            />
          )}
          </a>
        </section>
      </footer>
    );
  }
}

module.exports = Footer;
