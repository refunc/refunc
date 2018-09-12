module.exports = {
    head: [
        ['link', { rel: 'shortcut icon', href: `/icons/favicon.ico` }],
        ['link', { rel: 'icon', type: 'image/png', sizes: '96x96', href: `/icons/favicon-96x96.png` }],
        ['link', { rel: 'icon', type: 'image/png', sizes: '32x32', href: `/icons/favicon-32x32.png` }],
        ['link', { rel: 'icon', type: 'image/png', sizes: '16x16', href: `/icons/favicon-16x16.png` }],
        ['link', { rel: 'manifest', href: '/manifest.json' }],
        ['meta', { name: 'theme-color', content: '#3eaf7c' }],
        ['meta', { name: 'apple-mobile-web-app-capable', content: 'yes' }],
        ['meta', { name: 'apple-mobile-web-app-status-bar-style', content: 'black' }],
        ['link', { rel: 'apple-touch-icon', href: `/icons/apple-touch-icon-152x152.png` }],
        ['link', { rel: 'mask-icon', href: '/icons/safari-pinned-tab.svg', color: '#3eaf7c' }],
        ['meta', { name: 'msapplication-TileImage', content: '/icons/msapplication-icon-144x144.png' }],
        ['meta', { name: 'msapplication-TileColor', content: '#000000' }]
    ],
    base: '/',
    ga: 'UA-8653269-3',
    // serviceWorker: true,
    locales: {
        '/': {
            lang: 'zh-CN',
            title: 'Refunc',
            description: '基于Kubernetes的云函数平台'
        },
        '/en/': {
            lang: 'en-US',
            title: 'Refunc',
            description: 'A FaaS runs on kuberntes',
        },
    },
    themeConfig: {
        repo: 'refunc/refunc',
        editLinks: false,
        docsDir: 'docs',
        locales: {
            '/': {
                label: '简体中文',
                selectText: '选择语言',
                editLinkText: '在 GitHub 上编辑此页',
                lastUpdated: '上次更新',
                nav: [
                    {
                        text: '指南',
                        link: '/guide/',
                    },
                ],
                sidebar: {
                    '/guide/': genSidebarConfig('指南')
                },
            },
            '/en/': {
                label: 'English',
                selectText: 'Languages',
                editLinkText: 'Edit this page on GitHub',
                lastUpdated: 'Last Updated',
                nav: [
                    {
                        text: 'Guide',
                        link: '/en/guide/',
                    },
                ],
                sidebar: {
                    '/en/guide/': genSidebarConfig('Guide')
                },
            },
        },
    },
};

function genSidebarConfig(title) {
    return [
        {
            title,
            collapsable: false,
            children: [
                '',
                'quickstart',
                'concepts',
            ]
        }
    ]
}