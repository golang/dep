/* List of projects/orgs using your project for the users page */
const users = [
];

const siteConfig = {
  title: 'dep' /* title for your website */,
  tagline: 'Dependency management for Go',
  url: 'https://golang.github.io' /* your website url */,
  baseUrl: '/dep/' /* base url for your project */,
  editUrl: 'https://github.com/golang/dep/edit/master/docs/',
  projectName: 'dep',
  headerLinks: [
    {doc: 'introduction', label: 'Documentation'},
    {blog: true, label: 'Blog'},
  ],
  users,
  /* path to images for header/footer */
  headerIcon: 'docs/assets/DigbyFlat.svg',
  footerIcon: 'docs/assets/DigbyShadowsScene2.svg',
  favicon: 'docs/assets/DigbyScene2Flat.png',
  /* colors for website */
  colors: {
    secondaryColor: '#243f75',
    primaryColor: '#375eab',
  },
  algolia: {
    apiKey: "0b4cdbc6bb41efe17ed7176afcb23441",
    indexName: "golang_dep"
  },
  // This copyright info is used in /core/Footer.js and blog rss/atom feeds.
  copyright:
    'Copyright © ' +
    new Date().getFullYear() +
    ' The Go Authors',
   organizationName: 'golang', // or set an env variable ORGANIZATION_NAME
   projectName: 'dep', // or set an env variable PROJECT_NAME
  highlight: {
    // Highlight.js theme to use for syntax highlighting in code blocks
    theme: 'default',
  },
  scripts: ['https://buttons.github.io/buttons.js'],
  // You may provide arbitrary config keys to be used as needed by your template.
  repoUrl: 'https://github.com/golang/dep',
};

module.exports = siteConfig;
